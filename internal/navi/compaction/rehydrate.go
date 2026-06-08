package compaction

import (
	"fmt"
	"sort"
	"strings"
)

type Rehydrator struct{}

type rehydrationSection struct {
	kind      string
	label     string
	text      string
	protected bool
	order     int
}

func (Rehydrator) Assemble(input RehydrationInput) RehydrationOutput {
	protectedRecentWindow := input.ProtectedRecentWindow
	if protectedRecentWindow <= 0 {
		protectedRecentWindow = 4
	}
	maxTaskFrames := input.MaxTaskFrames
	if maxTaskFrames <= 0 {
		maxTaskFrames = 4
	}
	maxSupportSpans := input.MaxSupportSpans
	if maxSupportSpans <= 0 {
		maxSupportSpans = 3
	}

	sections := make([]rehydrationSection, 0, 16)
	order := 0
	metadata := RehydrationMetadata{}
	appendSection := func(kind, label, text string, protected bool) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		sections = append(sections, rehydrationSection{
			kind:      kind,
			label:     label,
			text:      text,
			protected: protected,
			order:     order,
		})
		if protected {
			metadata.ProtectedLabels = append(metadata.ProtectedLabels, label)
		}
		order++
	}

	for idx, instruction := range input.PermanentInstructions {
		appendSection("instructions", fmt.Sprintf("permanent_instruction_%d", idx+1), instruction, true)
	}

	appendSection("chat_core", "chat_core", renderChatCore(input.ChatFrame), true)
	appendSection("chat_detail", "chat_detail", renderChatDetail(input.ChatFrame), false)

	taskFrames := selectTaskFrames(input.TaskFrames, maxTaskFrames)
	for _, task := range taskFrames {
		kind := "task_active"
		protected := taskNeedsProtection(task)
		if !protected {
			kind = "task_resolved"
		}
		appendSection(kind, "task:"+task.TaskID, renderTaskFrame(task), protected)
		metadata.IncludedTaskIDs = append(metadata.IncludedTaskIDs, task.TaskID)
	}

	appendSection("proposal_refs", "proposal_refs", renderRefSection("## Active Proposals", input.ProposalRefs), len(input.ProposalRefs) > 0)
	appendSection("failure_refs", "failure_refs", renderRefSection("## Active Failures", input.FailureRefs), len(input.FailureRefs) > 0)

	for _, span := range selectRetrievalSpans(input.RetrievalSpans, maxSupportSpans) {
		appendSection("retrieval", "retrieval:"+firstNonEmpty(span.SpanID, span.Kind), renderRetrievalSpan(span), false)
		metadata.IncludedSupportSpanIDs = append(metadata.IncludedSupportSpanIDs, span.SpanID)
	}

	for idx, msg := range input.LiveTail {
		protected := idx >= len(input.LiveTail)-protectedRecentWindow
		appendSection("live_tail", "live_tail:"+firstNonEmpty(msg.ID, fmt.Sprintf("%d", idx)), renderLiveTailMessage(msg), protected)
	}

	appendSection("new_input", "new_input", renderNewInput(input.NewInput), strings.TrimSpace(input.NewInput) != "")

	totalCost := 0
	for _, section := range sections {
		totalCost += estimateSectionTokens(section.text)
	}
	dropped := make([]string, 0)
	usableTokens := input.Budget.UsableTokens
	if usableTokens > 0 && totalCost > usableTokens {
		dropByKind := func(kind, reason string) {
			for idx := range sections {
				if sections[idx].text == "" || sections[idx].protected || sections[idx].kind != kind {
					continue
				}
				dropped = append(dropped, sections[idx].label)
				metadata.DroppedSections = append(metadata.DroppedSections, DroppedSection{
					Label:  sections[idx].label,
					Kind:   sections[idx].kind,
					Reason: reason,
				})
				totalCost -= estimateSectionTokens(sections[idx].text)
				sections[idx].text = ""
				if totalCost <= usableTokens {
					return
				}
			}
		}

		dropByKind("retrieval", "trimmed_support_context_first")
		if totalCost > usableTokens {
			dropByKind("task_resolved", "trimmed_lower_value_task_detail")
		}
		if totalCost > usableTokens {
			dropByKind("chat_detail", "trimmed_lower_value_chat_detail")
		}
		if totalCost > usableTokens {
			for idx := range sections {
				if sections[idx].text == "" || sections[idx].protected || sections[idx].kind != "live_tail" {
					continue
				}
				dropped = append(dropped, sections[idx].label)
				metadata.DroppedSections = append(metadata.DroppedSections, DroppedSection{
					Label:  sections[idx].label,
					Kind:   sections[idx].kind,
					Reason: "trimmed_live_tail_after_support_and_detail",
				})
				totalCost -= estimateSectionTokens(sections[idx].text)
				sections[idx].text = ""
				if totalCost <= usableTokens {
					break
				}
			}
		}
	}

	out := make([]string, 0, len(sections))
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].order < sections[j].order })
	for _, section := range sections {
		if strings.TrimSpace(section.text) == "" {
			continue
		}
		out = append(out, section.text)
	}

	return RehydrationOutput{Sections: out, Dropped: dropped, Metadata: metadata}
}

func renderChatCore(frame ChatFrame) string {
	lines := []string{"## Session Continuity"}
	if objective := strings.TrimSpace(frame.PrimaryObjective); objective != "" {
		lines = append(lines, "Objective: "+objective)
	}
	if len(frame.ActiveTopics) > 0 {
		lines = append(lines, "Active topics: "+joinList(frame.ActiveTopics))
	}
	if len(frame.OpenQuestions) > 0 {
		lines = append(lines, "Open questions: "+joinList(frame.OpenQuestions))
	}
	if len(frame.ActiveCommitments) > 0 {
		lines = append(lines, "Commitments: "+joinList(frame.ActiveCommitments))
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderChatDetail(frame ChatFrame) string {
	lines := []string{"## Session State"}
	if len(frame.ActiveConstraints) > 0 {
		lines = append(lines, "Constraints: "+joinList(frame.ActiveConstraints))
	}
	if social := strings.TrimSpace(frame.SocialContinuity); social != "" {
		lines = append(lines, "Social continuity: "+social)
	}
	if len(frame.ActiveArtifactRefs) > 0 {
		lines = append(lines, "Active artifacts: "+joinList(frame.ActiveArtifactRefs))
	}
	if len(frame.SourceSpanRefs) > 0 {
		lines = append(lines, "Grounded by spans: "+joinList(frame.SourceSpanRefs))
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderTaskFrame(task TaskFrame) string {
	lines := []string{fmt.Sprintf("## Task %s [%s]", firstNonEmpty(strings.TrimSpace(task.Title), strings.TrimSpace(task.Objective), strings.TrimSpace(task.TaskID)), strings.TrimSpace(task.Status))}
	if objective := strings.TrimSpace(task.Objective); objective != "" && objective != strings.TrimSpace(task.Title) {
		lines = append(lines, "Objective: "+objective)
	}
	if phase := strings.TrimSpace(task.Phase); phase != "" {
		lines = append(lines, "Phase: "+phase)
	}
	if next := strings.TrimSpace(task.NextStep); next != "" {
		lines = append(lines, "Next step: "+next)
	}
	if len(task.Blockers) > 0 {
		lines = append(lines, "Blockers: "+joinList(task.Blockers))
	}
	if len(task.CurrentPlan) > 0 {
		lines = append(lines, "Current plan: "+joinList(limitList(task.CurrentPlan, 3)))
	}
	if len(task.CompletedSteps) > 0 {
		lines = append(lines, "Completed: "+joinList(limitList(task.CompletedSteps, 2)))
	}
	if len(task.Dependencies) > 0 {
		lines = append(lines, "Dependencies: "+joinList(limitList(task.Dependencies, 3)))
	}
	if len(task.ArtifactRefs) > 0 {
		lines = append(lines, "Artifacts: "+joinList(limitList(task.ArtifactRefs, 3)))
	}
	if len(task.ProposalRefs) > 0 {
		lines = append(lines, "Proposal refs: "+joinList(limitList(task.ProposalRefs, 3)))
	}
	if len(task.FailureRefs) > 0 {
		lines = append(lines, "Failure refs: "+joinList(limitList(task.FailureRefs, 3)))
	}
	return strings.Join(lines, "\n")
}

func renderRefSection(header string, refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	lines := []string{header}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		lines = append(lines, "- "+ref)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderRetrievalSpan(span RetrievalSpan) string {
	lines := []string{"## Supporting Context"}
	descriptor := strings.TrimSpace(span.Kind)
	if descriptor == "" {
		descriptor = "conversation"
	}
	if supportClass := strings.TrimSpace(span.SupportClass); supportClass != "" {
		descriptor += " / " + supportClass
	}
	if len(span.Tags) > 0 {
		descriptor += " [" + joinList(limitList(span.Tags, 3)) + "]"
	}
	lines = append(lines, "- "+descriptor+": "+trimForPrompt(span.Excerpt, 180))
	if len(span.ArtifactRefs) > 0 {
		lines = append(lines, "Artifacts: "+joinList(limitList(span.ArtifactRefs, 3)))
	}
	if len(span.RelatedTaskIDs) > 0 {
		lines = append(lines, "Related tasks: "+joinList(limitList(span.RelatedTaskIDs, 3)))
	}
	if span.Provenance.StartMessageID != "" || span.Provenance.EndMessageID != "" {
		lines = append(lines, "Provenance: "+firstNonEmpty(span.Provenance.StartMessageID, "?")+" -> "+firstNonEmpty(span.Provenance.EndMessageID, "?"))
	}
	return strings.Join(lines, "\n")
}

func renderLiveTailMessage(msg Message) string {
	content := trimForPrompt(msg.Content, 220)
	if content == "" {
		return ""
	}
	role := strings.TrimSpace(msg.Role)
	if role == "" {
		role = "message"
	}
	return fmt.Sprintf("## Recent Context\n%s: %s", role, content)
}

func renderNewInput(newInput string) string {
	newInput = strings.TrimSpace(newInput)
	if newInput == "" {
		return ""
	}
	return "## New User Input\n" + newInput
}

func selectTaskFrames(taskFrames []TaskFrame, maxTasks int) []TaskFrame {
	if len(taskFrames) == 0 {
		return nil
	}
	selected := append([]TaskFrame(nil), taskFrames...)
	sort.SliceStable(selected, func(i, j int) bool {
		left := taskPriority(selected[i])
		right := taskPriority(selected[j])
		if left != right {
			return left > right
		}
		return selected[i].TaskID < selected[j].TaskID
	})
	if maxTasks > 0 && len(selected) > maxTasks {
		selected = selected[:maxTasks]
	}
	return selected
}

func selectRetrievalSpans(spans []RetrievalSpan, max int) []RetrievalSpan {
	if len(spans) == 0 {
		return nil
	}
	selected := append([]RetrievalSpan(nil), spans...)
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Priority != selected[j].Priority {
			return selected[i].Priority > selected[j].Priority
		}
		if !selected[i].CreatedAt.Equal(selected[j].CreatedAt) {
			return selected[i].CreatedAt.After(selected[j].CreatedAt)
		}
		return selected[i].SpanID < selected[j].SpanID
	})
	if max > 0 && len(selected) > max {
		selected = selected[:max]
	}
	return selected
}

func taskPriority(task TaskFrame) int {
	score := 0
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "awaiting_user":
		score += 100
	case "blocked":
		score += 95
	case "active":
		score += 90
	case "paused":
		score += 60
	case "done":
		score += 10
	case "abandoned":
		score += 0
	default:
		score += 50
	}
	if strings.TrimSpace(task.NextStep) != "" {
		score += 15
	}
	if len(task.Blockers) > 0 {
		score += 15
	}
	if len(task.ProposalRefs) > 0 {
		score += 12
	}
	if len(task.FailureRefs) > 0 {
		score += 12
	}
	if len(task.CurrentPlan) > 0 {
		score += 6
	}
	return score
}

func taskNeedsProtection(task TaskFrame) bool {
	if len(task.ProposalRefs) > 0 || len(task.FailureRefs) > 0 || len(task.Blockers) > 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "active", "blocked", "awaiting_user":
		return true
	default:
		return false
	}
}

func estimateSectionTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(text)/4 + 8
}

func joinList(values []string) string {
	clean := compactNonEmpty(values)
	if len(clean) == 0 {
		return ""
	}
	return strings.Join(clean, "; ")
}

func compactNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func limitList(values []string, max int) []string {
	clean := compactNonEmpty(values)
	if max <= 0 || len(clean) <= max {
		return clean
	}
	return clean[:max]
}

func trimForPrompt(content string, max int) string {
	content = strings.TrimSpace(strings.Join(strings.Fields(content), " "))
	if max <= 0 || len(content) <= max {
		return content
	}
	if max <= 3 {
		return content[:max]
	}
	return content[:max-3] + "..."
}
