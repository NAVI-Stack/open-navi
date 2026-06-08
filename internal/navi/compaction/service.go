package compaction

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	Budget     BudgetManager
	Selector   SegmentSelector
	Rehydrator Rehydrator
	Memory     ChatMemoryStore
	Checkpoint CheckpointStore
}

type RunInput struct {
	ChatID             string
	Messages              []Message
	PermanentInstructions []string
	ProposalRefs          []string
	FailureRefs           []string
	RuntimeSnapshots      []RunSnapshot
	NewInput              string
	MaxContextTokens      int
	ReservedOutputTokens  int
	ReservedToolHeadroom  int
}

type RunOutput struct {
	Triggered   TriggerClass
	Checkpoint  *CompactionCheckpoint
	Rehydration RehydrationOutput
	Inspection  CompactionInspection
}

type CompactionInspection struct {
	Triggered      TriggerClass         `json:"triggered,omitempty"`
	TriggerReason  string               `json:"trigger_reason,omitempty"`
	Selection      CompactionSelection  `json:"selection,omitempty"`
	TaskSurvival   []TaskSurvivalRecord `json:"task_survival,omitempty"`
	SupportSpans   []RetrievalSpan      `json:"support_spans,omitempty"`
	SelectedSpan   SourceSpanProvenance `json:"selected_span,omitempty"`
	SourceMessages []string             `json:"source_messages,omitempty"`
}

func (s Service) Run(ctx context.Context, in RunInput) (RunOutput, error) {
	out := RunOutput{}
	if strings.TrimSpace(in.ChatID) == "" {
		return out, nil
	}
	snapshot := s.Budget.Snapshot(in.MaxContextTokens, in.ReservedOutputTokens, in.ReservedToolHeadroom, in.Messages)
	trigger := s.Budget.TriggerClass(snapshot)
	out.Triggered = trigger
	out.Inspection.Triggered = trigger
	out.Inspection.TriggerReason = triggerReason(snapshot, trigger)

	runSnapshots := append([]RunSnapshot(nil), in.RuntimeSnapshots...)
	if len(runSnapshots) == 0 {
		if provider, ok := s.Memory.(RuntimeStateStore); ok {
			runSnapshots, _ = provider.ListCompactionRunSnapshots(ctx, in.ChatID)
		}
	}

	mem, _ := s.Memory.GetChatMemory(ctx, in.ChatID)
	if mem.ChatID == "" {
		mem = ChatMemory{
			ChatID:      in.ChatID,
			SchemaVersion:  1,
			MemoryVersion:  0,
			ChatFrame:   ChatFrame{SchemaVersion: 1, ChatID: in.ChatID},
			RetrievalSpans: nil,
		}
	}
	if trigger == "" {
		out.Rehydration = s.Rehydrator.Assemble(RehydrationInput{
			PermanentInstructions: in.PermanentInstructions,
			ChatFrame:          mem.ChatFrame,
			TaskFrames:            mem.TaskFrames,
			ProposalRefs:          in.ProposalRefs,
			FailureRefs:           in.FailureRefs,
			RetrievalSpans:        mem.RetrievalSpans,
			LiveTail:              in.Messages,
			NewInput:              in.NewInput,
			Budget:                snapshot,
			ProtectedRecentWindow: 4,
			MaxTaskFrames:         6,
			MaxSupportSpans:       4,
		})
		return out, nil
	}

	selection, ok := s.Selector.Select(SelectorInput{
		Messages:         in.Messages,
		Trigger:          trigger,
		RuntimeSnapshots: runSnapshots,
	})
	out.Inspection.Selection = selection.Trace
	if !ok {
		out.Rehydration = s.Rehydrator.Assemble(RehydrationInput{
			PermanentInstructions: in.PermanentInstructions,
			ChatFrame:          mem.ChatFrame,
			TaskFrames:            mem.TaskFrames,
			ProposalRefs:          in.ProposalRefs,
			FailureRefs:           in.FailureRefs,
			RetrievalSpans:        mem.RetrievalSpans,
			LiveTail:              in.Messages,
			NewInput:              in.NewInput,
			Budget:                snapshot,
			ProtectedRecentWindow: 4,
			MaxTaskFrames:         6,
			MaxSupportSpans:       4,
		})
		return out, nil
	}

	selectedSpan := provenanceFromMessages(in.ChatID, selection.Messages, in.ProposalRefs, in.FailureRefs, artifactRefsForMessages(selection.Messages, runSnapshots))
	out.Inspection.SelectedSpan = selectedSpan
	out.Inspection.SourceMessages = append([]string(nil), selectedSpan.SourceMessageIDs...)

	incomingFrame := buildIncomingChatFrame(mem.ChatFrame, in, selection.Messages, runSnapshots, selectedSpan)
	mergedFrame := MergeChatFrame(mem.ChatFrame, incomingFrame)
	SyncAuthoritativeRefs(&mergedFrame, in.ProposalRefs, in.FailureRefs)

	mem.MemoryVersion++
	mem.CurrentEpochID = uuid.NewString()
	mem.ChatFrame = mergedFrame
	extractedTasks := extractTaskFrames(selection.Messages, runSnapshots, in.ProposalRefs, in.FailureRefs)
	mem.TaskFrames, out.Inspection.TaskSurvival = MergeTaskFramesDetailed(mem.TaskFrames, extractedTasks)
	supportSpans := retrievalSpansFromSelection(in.ChatID, mem.CurrentEpochID, selection.Messages, mem.TaskFrames, runSnapshots)
	mem.RetrievalSpans = MergeRetrievalSpans(mem.RetrievalSpans, supportSpans)
	mem.UpdatedAt = time.Now().UTC()

	if err := s.Memory.PutChatMemory(ctx, mem); err != nil {
		return out, fmt.Errorf("persist memory: %w", err)
	}

	cp := CompactionCheckpoint{
		CheckpointID:            uuid.NewString(),
		ChatID:               in.ChatID,
		EpochID:                 mem.CurrentEpochID,
		TriggerClass:            trigger,
		TriggerReason:           out.Inspection.TriggerReason,
		CompactedMessageStartID: selection.Messages[0].ID,
		CompactedMessageEndID:   selection.Messages[len(selection.Messages)-1].ID,
		ChatFrameVersion:     mergedFrame.FrameVersion,
		TaskFrameVersions:       taskFrameVersions(mem.TaskFrames),
		SourceMessageIDs:        append([]string(nil), selectedSpan.SourceMessageIDs...),
		SelectedSpan:            selectedSpan,
		SelectionTrace:          selection.Trace,
		TaskSurvival:            append([]TaskSurvivalRecord(nil), out.Inspection.TaskSurvival...),
		RetrievalSpanIDs:        retrievalSpanIDs(supportSpans),
		EstimatorSnapshot:       snapshot,
		CreatedAt:               time.Now().UTC(),
	}
	if err := s.Checkpoint.AppendCompactionCheckpoint(ctx, cp); err != nil {
		return out, fmt.Errorf("checkpoint: %w", err)
	}
	if err := s.Checkpoint.MarkMessagesCompacted(ctx, in.ChatID, cp.CheckpointID, cp.EpochID, cp.CompactedMessageStartID, cp.CompactedMessageEndID); err != nil {
		return out, fmt.Errorf("mark compacted: %w", err)
	}

	out.Checkpoint = &cp
	out.Inspection.SupportSpans = append([]RetrievalSpan(nil), supportSpans...)
	out.Rehydration = s.Rehydrator.Assemble(RehydrationInput{
		PermanentInstructions: in.PermanentInstructions,
		ChatFrame:          mem.ChatFrame,
		TaskFrames:            mem.TaskFrames,
		ProposalRefs:          in.ProposalRefs,
		FailureRefs:           in.FailureRefs,
		RetrievalSpans:        mem.RetrievalSpans,
		LiveTail:              in.Messages[selection.EndIndex+1:],
		NewInput:              in.NewInput,
		Budget:                snapshot,
		ProtectedRecentWindow: 4,
		MaxTaskFrames:         6,
		MaxSupportSpans:       4,
	})
	return out, nil
}

var (
	taskIDPattern        = regexp.MustCompile(`(?i)\[task:([a-zA-Z0-9._:-]+)\]`)
	blockerPattern       = regexp.MustCompile(`(?i)\bblocker:\s*(.+)$`)
	dependencyPattern    = regexp.MustCompile(`(?i)\bdependency:\s*(.+)$`)
	nextStepPattern      = regexp.MustCompile(`(?i)\bnext step:\s*(.+)$`)
	completedStepPattern = regexp.MustCompile(`(?i)\bcompleted:\s*(.+)$`)
	planStepPattern      = regexp.MustCompile(`(?i)\bplan:\s*(.+)$`)
	proposalLinkPattern  = regexp.MustCompile(`(?i)\[proposal:([a-zA-Z0-9._:/-]+)\]`)
	failureLinkPattern   = regexp.MustCompile(`(?i)\[failure:([a-zA-Z0-9._:/-]+)\]`)
	dependencyTagPattern = regexp.MustCompile(`(?i)\[depends:([a-zA-Z0-9._:/-]+)\]`)
)

func buildIncomingChatFrame(existing ChatFrame, in RunInput, selected []Message, runs []RunSnapshot, selectedSpan SourceSpanProvenance) ChatFrame {
	frame := ChatFrame{
		ChatID:          in.ChatID,
		CurrentEpochID:     existing.CurrentEpochID,
		PrimaryObjective:   firstNonEmpty(existing.PrimaryObjective, strings.TrimSpace(in.NewInput), firstObjectiveFromRuns(runs), firstContentLine(joinSelectedContent(selected))),
		ActiveArtifactRefs: artifactRefsForMessages(selected, runs),
		SourceSpanRefs:     []string{selectedSpan.Ref},
		Provenance:         []SourceSpanProvenance{selectedSpan},
	}
	for _, run := range runs {
		if goal := strings.TrimSpace(run.Scratchpad["goal"]); goal != "" {
			frame.ActiveCommitments = union(frame.ActiveCommitments, []string{goal})
		}
		if phase := strings.TrimSpace(run.CurrentPhase); phase != "" {
			frame.ActiveTopics = union(frame.ActiveTopics, []string{"run_phase:" + phase})
		}
	}
	return frame
}

func extractTaskFrames(messages []Message, runs []RunSnapshot, proposalRefs, failureRefs []string) []TaskFrame {
	taskMap := map[string]TaskFrame{}
	for _, run := range runs {
		task := taskFrameFromRunSnapshot(run)
		if task.TaskID == "" {
			continue
		}
		task.ProposalRefs = union(task.ProposalRefs, proposalRefs)
		task.FailureRefs = union(task.FailureRefs, failureRefs)
		taskMap[task.TaskID] = task
	}
	for _, msg := range messages {
		taskID := detectTaskID(msg)
		if taskID == "" {
			continue
		}
		task := taskMap[taskID]
		if task.TaskID == "" {
			task = TaskFrame{
				TaskID:         taskID,
				RunID:          strings.TrimSpace(msg.RunID),
				FrameVersion:   1,
				Title:          firstContentLine(msg.Content),
				Objective:      firstContentLine(msg.Content),
				Status:         "active",
				CurrentPlan:    []string{},
				Blockers:       []string{},
				Dependencies:   []string{},
				IdentitySource: "message",
				LastUpdatedAt:  msg.CreatedAt,
			}
		}
		updateTaskFrameFromMessage(&task, msg)
		task.ProposalRefs = union(task.ProposalRefs, proposalRefs)
		task.FailureRefs = union(task.FailureRefs, failureRefs)
		taskMap[taskID] = task
	}
	frames := make([]TaskFrame, 0, len(taskMap))
	for _, tf := range taskMap {
		if tf.FrameVersion == 0 {
			tf.FrameVersion = 1
		}
		frames = append(frames, tf)
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].TaskID < frames[j].TaskID })
	return frames
}

func taskFrameFromRunSnapshot(run RunSnapshot) TaskFrame {
	runID := strings.TrimSpace(run.RunID)
	if runID == "" {
		return TaskFrame{}
	}
	task := TaskFrame{
		TaskID:         runID,
		RunID:          runID,
		CheckpointID:   strings.TrimSpace(run.LatestCheckpointID),
		FrameVersion:   1,
		Title:          firstNonEmpty(run.Scratchpad["current_task"], run.Scratchpad["goal"], runID),
		Objective:      firstNonEmpty(run.Scratchpad["goal"], run.Scratchpad["current_task"], runID),
		Status:         taskStatusFromRun(run),
		Phase:          strings.TrimSpace(run.CurrentPhase),
		Blockers:       []string{},
		Dependencies:   []string{},
		CurrentPlan:    []string{},
		CompletedSteps: []string{},
		LinkedEntities: []string{},
		ArtifactRefs:   artifactRefsForRun(run),
		ProposalRefs:   compactNonEmpty([]string{strings.TrimSpace(run.BlockedOnProposalID), strings.TrimSpace(run.PendingProposalID)}),
		FailureRefs:    nil,
		SourceSpanRefs: compactNonEmpty([]string{strings.TrimSpace(run.LatestCheckpointID)}),
		IdentitySource: "runtime",
		StatusSource:   "runtime",
		LastUpdatedAt:  laterTime(run.CheckpointCreatedAt, run.UpdatedAt),
		Provenance: []SourceSpanProvenance{{
			Ref:            firstNonEmpty(strings.TrimSpace(run.LatestCheckpointID), "run:"+runID),
			RunIDs:         []string{runID},
			ProposalRefs:   compactNonEmpty([]string{strings.TrimSpace(run.BlockedOnProposalID), strings.TrimSpace(run.PendingProposalID)}),
			ArtifactRefs:   artifactRefsForRun(run),
			CheckpointRefs: compactNonEmpty([]string{strings.TrimSpace(run.LatestCheckpointID)}),
		}},
	}
	if reason := strings.TrimSpace(run.PendingProposalReason); reason != "" {
		task.Blockers = append(task.Blockers, "awaiting proposal: "+reason)
	}
	if step := strings.TrimSpace(run.Scratchpad["current_step"]); step != "" {
		task.NextStep = step
	}
	if task.NextStep == "" && strings.TrimSpace(run.BlockedOnProposalID) != "" {
		task.NextStep = "Await proposal resolution for " + strings.TrimSpace(run.BlockedOnProposalID)
	}
	if task.NextStep == "" && strings.TrimSpace(run.PendingProposalID) != "" {
		task.NextStep = "Await proposal resolution for " + strings.TrimSpace(run.PendingProposalID)
	}
	return task
}

func taskStatusFromRun(run RunSnapshot) string {
	status := strings.ToLower(strings.TrimSpace(run.Status))
	switch {
	case strings.TrimSpace(run.BlockedOnProposalID) != "", strings.TrimSpace(run.PendingProposalID) != "":
		return "awaiting_user"
	case strings.Contains(status, "paused"), strings.Contains(status, "waiting"):
		return "paused"
	case strings.Contains(status, "failed"):
		return "blocked"
	case strings.Contains(status, "completed"), strings.Contains(status, "cancelled"):
		return "done"
	default:
		return "active"
	}
}
func detectTaskID(msg Message) string {
	if strings.TrimSpace(msg.RunID) != "" {
		return strings.TrimSpace(msg.RunID)
	}
	if match := taskIDPattern.FindStringSubmatch(msg.Content); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	kind := strings.TrimSpace(strings.ToLower(msg.MessageKind))
	if strings.Contains(kind, "task:") {
		parts := strings.Split(kind, "task:")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return ""
}

func updateTaskFrameFromMessage(task *TaskFrame, msg Message) {
	if task == nil {
		return
	}
	content := strings.TrimSpace(msg.Content)
	kind := strings.ToLower(strings.TrimSpace(msg.MessageKind))
	task.RunID = firstNonEmpty(task.RunID, strings.TrimSpace(msg.RunID))
	task.LastUpdatedAt = laterTime(task.LastUpdatedAt, msg.CreatedAt)
	task.SourceSpanRefs = union(task.SourceSpanRefs, []string{msg.ID})
	task.Provenance = mergeProvenance(task.Provenance, []SourceSpanProvenance{{
		Ref:              msg.ID,
		StartMessageID:   msg.ID,
		EndMessageID:     msg.ID,
		SourceMessageIDs: []string{msg.ID},
		RunIDs:           compactNonEmpty([]string{strings.TrimSpace(msg.RunID)}),
		ProposalRefs:     extractAllMatches(content, proposalLinkPattern),
		FailureRefs:      extractAllMatches(content, failureLinkPattern),
		ArtifactRefs:     compactNonEmpty([]string{extractPathHint(content)}),
	}})
	switch {
	case strings.Contains(kind, "blocked") || blockerPattern.MatchString(content):
		task.Status = "blocked"
		task.StatusSource = "message"
		if match := blockerPattern.FindStringSubmatch(content); len(match) == 2 {
			task.Blockers = union(task.Blockers, []string{strings.TrimSpace(match[1])})
		}
	case strings.Contains(kind, "awaiting_user"):
		task.Status = "awaiting_user"
		task.StatusSource = "message"
	case strings.Contains(kind, "done") || strings.Contains(kind, "completed"):
		task.Status = "done"
		task.StatusSource = "message"
		task.NextStep = ""
	default:
		if task.Status == "" {
			task.Status = "active"
			task.StatusSource = "message"
		}
	}
	if match := nextStepPattern.FindStringSubmatch(content); len(match) == 2 {
		task.NextStep = strings.TrimSpace(match[1])
	}
	if match := dependencyPattern.FindStringSubmatch(content); len(match) == 2 {
		task.Dependencies = union(task.Dependencies, []string{strings.TrimSpace(match[1])})
	}
	task.Dependencies = union(task.Dependencies, extractAllMatches(content, dependencyTagPattern))
	if match := completedStepPattern.FindStringSubmatch(content); len(match) == 2 {
		task.CompletedSteps = union(task.CompletedSteps, []string{strings.TrimSpace(match[1])})
	}
	if match := planStepPattern.FindStringSubmatch(content); len(match) == 2 {
		task.CurrentPlan = union(task.CurrentPlan, []string{strings.TrimSpace(match[1])})
	} else if content != "" && msg.Role == "navi" && !strings.Contains(strings.ToLower(kind), "done") {
		task.CurrentPlan = union(task.CurrentPlan, []string{firstContentLine(content)})
	}
	task.ArtifactRefs = union(task.ArtifactRefs, compactNonEmpty([]string{extractPathHint(content)}))
	task.ProposalRefs = union(task.ProposalRefs, extractAllMatches(content, proposalLinkPattern))
	task.FailureRefs = union(task.FailureRefs, extractAllMatches(content, failureLinkPattern))
	if task.Title == "" {
		task.Title = firstContentLine(content)
	}
	if task.Objective == "" {
		task.Objective = firstContentLine(content)
	}
}

func taskFrameVersions(frames []TaskFrame) map[string]int {
	versions := make(map[string]int, len(frames))
	for _, tf := range frames {
		versions[tf.TaskID] = maxInt(tf.FrameVersion, len(tf.CurrentPlan)+len(tf.CompletedSteps)+len(tf.Blockers)+len(tf.Dependencies))
	}
	return versions
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func retrievalSpansFromSelection(chatID, checkpointID string, messages []Message, tasks []TaskFrame, runs []RunSnapshot) []RetrievalSpan {
	if len(messages) == 0 {
		return nil
	}
	grouped := groupMessagesForSupport(messages, runs)
	out := make([]RetrievalSpan, 0, len(grouped))
	for idx, group := range grouped {
		provenance := provenanceFromMessages(chatID, group, nil, nil, artifactRefsForMessages(group, runs))
		out = append(out, RetrievalSpan{
			SpanID:           fmt.Sprintf("%s:support:%d", firstNonEmpty(checkpointID, chatID), idx+1),
			ChatID:        chatID,
			CheckpointID:     checkpointID,
			Kind:             supportKindForGroup(group, runs),
			SupportClass:     supportClassForGroup(group, runs),
			SourceMessageIDs: append([]string(nil), provenance.SourceMessageIDs...),
			Excerpt:          excerptForMessages(group),
			ArtifactRefs:     provenance.ArtifactRefs,
			RelatedTaskIDs:   relatedTaskIDs(group, tasks),
			Tags:             supportTagsForGroup(group, runs),
			Priority:         supportPriorityForGroup(group, runs),
			Provenance:       provenance,
			CreatedAt:        time.Now().UTC(),
		})
	}
	return out
}

func groupMessagesForSupport(messages []Message, runs []RunSnapshot) [][]Message {
	if len(messages) == 0 {
		return nil
	}
	var out [][]Message
	current := []Message{messages[0]}
	currentClass := supportClassForMessage(messages[0], runs)
	for _, msg := range messages[1:] {
		class := supportClassForMessage(msg, runs)
		if class != currentClass || strings.TrimSpace(msg.RunID) != strings.TrimSpace(current[len(current)-1].RunID) {
			out = append(out, current)
			current = []Message{msg}
			currentClass = class
			continue
		}
		current = append(current, msg)
	}
	out = append(out, current)
	return out
}

func supportClassForMessage(msg Message, runs []RunSnapshot) string {
	kind := normalizeKind(msg.MessageKind)
	switch kind {
	case "proposal_open", "proposal_resolution":
		return "decision_support"
	case "failure_open", "recovery_active", "recovery_resolved":
		return "recovery_support"
	case "tool_call", "tool_result":
		return "execution_support"
	case "artifact_mutation", "artifact_mutation_settled":
		return "artifact_support"
	default:
		if hasRunPendingProposal(strings.TrimSpace(msg.RunID), runs) {
			return "decision_support"
		}
		return "conversation_support"
	}
}

func supportKindForGroup(messages []Message, runs []RunSnapshot) string {
	switch supportClassForGroup(messages, runs) {
	case "decision_support":
		return "decision_support"
	case "recovery_support":
		return "conversation"
	case "execution_support":
		return "tool_output"
	case "artifact_support":
		return "artifact_context"
	default:
		return "conversation"
	}
}

func supportClassForGroup(messages []Message, runs []RunSnapshot) string {
	if len(messages) == 0 {
		return "conversation_support"
	}
	return supportClassForMessage(messages[len(messages)-1], runs)
}

func supportTagsForGroup(messages []Message, runs []RunSnapshot) []string {
	if len(messages) == 0 {
		return nil
	}
	return compactNonEmpty([]string{
		supportClassForGroup(messages, runs),
		normalizeKind(messages[len(messages)-1].MessageKind),
	})
}

func supportPriorityForGroup(messages []Message, runs []RunSnapshot) int {
	switch supportClassForGroup(messages, runs) {
	case "decision_support", "recovery_support":
		return 90
	case "artifact_support":
		return 80
	case "execution_support":
		return 70
	default:
		return 50
	}
}

func relatedTaskIDs(messages []Message, tasks []TaskFrame) []string {
	taskSet := map[string]struct{}{}
	for _, msg := range messages {
		if taskID := detectTaskID(msg); taskID != "" {
			taskSet[taskID] = struct{}{}
		}
	}
	out := make([]string, 0, len(taskSet))
	for _, task := range tasks {
		if _, ok := taskSet[task.TaskID]; ok {
			out = append(out, task.TaskID)
		}
	}
	sort.Strings(out)
	return out
}

func provenanceFromMessages(chatID string, messages []Message, proposalRefs, failureRefs, artifactRefs []string) SourceSpanProvenance {
	if len(messages) == 0 {
		return SourceSpanProvenance{}
	}
	runIDs := make([]string, 0, len(messages))
	messageIDs := make([]string, 0, len(messages))
	for _, msg := range messages {
		messageIDs = append(messageIDs, msg.ID)
		runIDs = append(runIDs, strings.TrimSpace(msg.RunID))
	}
	ref := fmt.Sprintf("%s:%s:%s", chatID, messages[0].ID, messages[len(messages)-1].ID)
	return SourceSpanProvenance{
		Ref:              ref,
		StartMessageID:   messages[0].ID,
		EndMessageID:     messages[len(messages)-1].ID,
		SourceMessageIDs: compactNonEmpty(messageIDs),
		RunIDs:           compactNonEmpty(runIDs),
		ProposalRefs:     compactNonEmpty(proposalRefs),
		FailureRefs:      compactNonEmpty(failureRefs),
		ArtifactRefs:     compactNonEmpty(artifactRefs),
	}
}

func excerptForMessages(messages []Message) string {
	lines := make([]string, 0, minInt(len(messages), 4))
	for idx, msg := range messages {
		if idx >= 4 {
			break
		}
		line := strings.TrimSpace(firstContentLine(msg.Content))
		if line == "" {
			continue
		}
		lines = append(lines, msg.Role+": "+line)
	}
	return strings.Join(lines, " | ")
}

func artifactRefsForMessages(messages []Message, runs []RunSnapshot) []string {
	out := []string{}
	runIndex := map[string]RunSnapshot{}
	for _, run := range runs {
		runIndex[strings.TrimSpace(run.RunID)] = run
	}
	for _, msg := range messages {
		out = union(out, compactNonEmpty([]string{extractPathHint(msg.Content)}))
		if run, ok := runIndex[strings.TrimSpace(msg.RunID)]; ok {
			out = union(out, artifactRefsForRun(run))
		}
	}
	return out
}

func artifactRefsForRun(run RunSnapshot) []string {
	values := append([]string{strings.TrimSpace(run.MainArtifactID), strings.TrimSpace(run.CheckpointMainArtifact)}, run.ArtifactIDs...)
	values = append(values, run.CheckpointArtifactIDs...)
	return compactNonEmpty(values)
}

func joinSelectedContent(messages []Message) string {
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		if line := firstContentLine(msg.Content); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func firstContentLine(content string) string {
	line := strings.TrimSpace(content)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return strings.TrimSpace(taskIDPattern.ReplaceAllString(line, ""))
}

func firstObjectiveFromRuns(runs []RunSnapshot) string {
	for _, run := range runs {
		if goal := strings.TrimSpace(run.Scratchpad["goal"]); goal != "" {
			return goal
		}
	}
	return ""
}

func triggerReason(snapshot BudgetSnapshot, trigger TriggerClass) string {
	switch trigger {
	case TriggerEmergency:
		return fmt.Sprintf("estimated prompt tokens %d exceeded emergency threshold %d", snapshot.EstimatedPromptTokens, snapshot.EmergencyThreshold)
	case TriggerHard:
		return fmt.Sprintf("estimated prompt tokens %d exceeded hard threshold %d", snapshot.EstimatedPromptTokens, snapshot.HardThreshold)
	case TriggerSoft:
		return fmt.Sprintf("estimated prompt tokens %d exceeded soft threshold %d", snapshot.EstimatedPromptTokens, snapshot.SoftThreshold)
	default:
		return ""
	}
}

func retrievalSpanIDs(spans []RetrievalSpan) []string {
	out := make([]string, 0, len(spans))
	for _, span := range spans {
		if strings.TrimSpace(span.SpanID) != "" {
			out = append(out, span.SpanID)
		}
	}
	return out
}

func extractAllMatches(content string, pattern *regexp.Regexp) []string {
	matches := pattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	return compactNonEmpty(out)
}

func hasRunPendingProposal(runID string, runs []RunSnapshot) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	for _, run := range runs {
		if strings.TrimSpace(run.RunID) == runID {
			return strings.TrimSpace(run.PendingProposalID) != "" || strings.TrimSpace(run.BlockedOnProposalID) != ""
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
