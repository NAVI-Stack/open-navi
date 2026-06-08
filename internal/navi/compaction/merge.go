package compaction

import (
	"sort"
	"strings"
	"time"
)

func MergeChatFrame(existing ChatFrame, incoming ChatFrame) ChatFrame {
	merged := existing
	if merged.SchemaVersion == 0 {
		merged.SchemaVersion = 1
	}
	merged.FrameVersion++
	merged.ChatID = firstNonEmpty(existing.ChatID, incoming.ChatID)
	merged.CurrentEpochID = firstNonEmpty(incoming.CurrentEpochID, existing.CurrentEpochID)
	if len(incoming.PrimaryObjective) > len(existing.PrimaryObjective) {
		merged.PrimaryObjective = incoming.PrimaryObjective
	}
	merged.ActiveTopics = union(existing.ActiveTopics, incoming.ActiveTopics)
	merged.OpenQuestions = union(existing.OpenQuestions, incoming.OpenQuestions)
	merged.ActiveConstraints = union(existing.ActiveConstraints, incoming.ActiveConstraints)
	merged.ActiveCommitments = union(existing.ActiveCommitments, incoming.ActiveCommitments)
	merged.ActiveArtifactRefs = union(existing.ActiveArtifactRefs, incoming.ActiveArtifactRefs)
	merged.ActiveProposalRefs = union(existing.ActiveProposalRefs, incoming.ActiveProposalRefs)
	merged.ActiveFailureRefs = union(existing.ActiveFailureRefs, incoming.ActiveFailureRefs)
	merged.SourceSpanRefs = union(existing.SourceSpanRefs, incoming.SourceSpanRefs)
	merged.Provenance = mergeProvenance(existing.Provenance, incoming.Provenance)
	return merged
}

func MergeTaskFrames(existing, incoming []TaskFrame) []TaskFrame {
	merged, _ := MergeTaskFramesDetailed(existing, incoming)
	return merged
}

func MergeTaskFramesDetailed(existing, incoming []TaskFrame) ([]TaskFrame, []TaskSurvivalRecord) {
	index := map[string]TaskFrame{}
	for _, tf := range existing {
		if tf.TaskID == "" {
			continue
		}
		index[tf.TaskID] = tf
	}
	survival := make([]TaskSurvivalRecord, 0, len(incoming))
	for _, tf := range incoming {
		if tf.TaskID == "" {
			continue
		}
		if current, ok := index[tf.TaskID]; ok {
			index[tf.TaskID] = mergeSingleTaskFrame(current, tf)
			survival = append(survival, taskSurvivalRecord(index[tf.TaskID], "merged_with_existing"))
			continue
		}
		tf.FrameVersion = maxInt(tf.FrameVersion, 1)
		index[tf.TaskID] = tf
		survival = append(survival, taskSurvivalRecord(tf, "new_or_rehydrated"))
	}
	out := make([]TaskFrame, 0, len(index))
	for _, tf := range index {
		out = append(out, tf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID < out[j].TaskID })
	sort.SliceStable(survival, func(i, j int) bool {
		if survival[i].TaskID != survival[j].TaskID {
			return survival[i].TaskID < survival[j].TaskID
		}
		return survival[i].Reason < survival[j].Reason
	})
	return out, survival
}

func MergeRetrievalSpans(existing, incoming []RetrievalSpan) []RetrievalSpan {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	index := map[string]RetrievalSpan{}
	for _, span := range existing {
		if span.SpanID != "" {
			index[span.SpanID] = span
		}
	}
	for _, span := range incoming {
		if span.SpanID == "" {
			continue
		}
		current, ok := index[span.SpanID]
		if !ok {
			index[span.SpanID] = span
			continue
		}
		current.ArtifactRefs = union(current.ArtifactRefs, span.ArtifactRefs)
		current.RelatedTaskIDs = union(current.RelatedTaskIDs, span.RelatedTaskIDs)
		current.Tags = union(current.Tags, span.Tags)
		current.Priority = maxInt(current.Priority, span.Priority)
		current.Provenance = mergeSingleProvenance(current.Provenance, span.Provenance)
		if len(strings.TrimSpace(span.Excerpt)) > len(strings.TrimSpace(current.Excerpt)) {
			current.Excerpt = span.Excerpt
		}
		if span.CreatedAt.After(current.CreatedAt) {
			current.CreatedAt = span.CreatedAt
		}
		current.Kind = firstNonEmpty(span.Kind, current.Kind)
		current.SupportClass = firstNonEmpty(span.SupportClass, current.SupportClass)
		current.SourceMessageIDs = union(current.SourceMessageIDs, span.SourceMessageIDs)
		index[span.SpanID] = current
	}
	out := make([]RetrievalSpan, 0, len(index))
	for _, span := range index {
		out = append(out, span)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].SpanID < out[j].SpanID
	})
	return out
}

func SyncAuthoritativeRefs(frame *ChatFrame, proposalRefs, failureRefs []string) {
	if frame == nil {
		return
	}
	frame.ActiveProposalRefs = append([]string(nil), proposalRefs...)
	frame.ActiveFailureRefs = append([]string(nil), failureRefs...)
}

func union(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, item := range append(append([]string(nil), a...), b...) {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func mergeSingleTaskFrame(existing, incoming TaskFrame) TaskFrame {
	merged := existing
	merged.FrameVersion = maxInt(existing.FrameVersion, incoming.FrameVersion) + 1
	merged.TaskID = firstNonEmpty(existing.TaskID, incoming.TaskID)
	merged.RunID = firstNonEmpty(incoming.RunID, existing.RunID)
	merged.CheckpointID = firstNonEmpty(incoming.CheckpointID, existing.CheckpointID)
	merged.ParentTaskID = firstNonEmpty(existing.ParentTaskID, incoming.ParentTaskID)
	merged.Title = preferredText(existing.Title, incoming.Title)
	merged.Objective = preferredText(existing.Objective, incoming.Objective)
	if strings.TrimSpace(incoming.Status) != "" || strings.TrimSpace(existing.Status) == "" || authoritativeTaskUpdate(incoming) {
		merged.Status = firstNonEmpty(incoming.Status, existing.Status)
		merged.Phase = firstNonEmpty(incoming.Phase, existing.Phase)
		merged.StatusSource = firstNonEmpty(incoming.StatusSource, existing.StatusSource)
	}
	merged.Blockers = union(existing.Blockers, incoming.Blockers)
	merged.Dependencies = union(existing.Dependencies, incoming.Dependencies)
	merged.LinkedEntities = union(existing.LinkedEntities, incoming.LinkedEntities)
	merged.ArtifactRefs = union(existing.ArtifactRefs, incoming.ArtifactRefs)
	merged.ProposalRefs = union(existing.ProposalRefs, incoming.ProposalRefs)
	merged.FailureRefs = union(existing.FailureRefs, incoming.FailureRefs)
	merged.SourceSpanRefs = union(existing.SourceSpanRefs, incoming.SourceSpanRefs)
	merged.Provenance = mergeProvenance(existing.Provenance, incoming.Provenance)
	merged.IdentitySource = firstNonEmpty(incoming.IdentitySource, existing.IdentitySource)
	merged.LastUpdatedAt = laterTime(existing.LastUpdatedAt, incoming.LastUpdatedAt)
	if shouldReplaceTaskField(existing.NextStep, incoming.NextStep, authoritativeTaskUpdate(incoming), incoming.Status) {
		merged.NextStep = strings.TrimSpace(incoming.NextStep)
	}
	if len(incoming.CurrentPlan) > 0 || authoritativeTaskUpdate(incoming) {
		merged.CurrentPlan = append([]string(nil), compactNonEmpty(incoming.CurrentPlan)...)
	}
	if len(incoming.CompletedSteps) > 0 {
		merged.CompletedSteps = union(existing.CompletedSteps, incoming.CompletedSteps)
	}
	return merged
}

func authoritativeTaskUpdate(task TaskFrame) bool {
	return strings.TrimSpace(task.RunID) != "" || strings.TrimSpace(task.CheckpointID) != "" || len(task.Provenance) > 0
}

func preferredText(existing, incoming string) string {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	switch {
	case incoming == "":
		return existing
	case existing == "":
		return incoming
	case len(incoming) > len(existing):
		return incoming
	default:
		return existing
	}
}

func shouldReplaceTaskField(existing, incoming string, authoritative bool, status string) bool {
	if strings.TrimSpace(incoming) != "" {
		return true
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if authoritative && (status == "done" || status == "abandoned") {
		return true
	}
	return false
}

func mergeProvenance(existing, incoming []SourceSpanProvenance) []SourceSpanProvenance {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	index := map[string]SourceSpanProvenance{}
	keyFor := func(p SourceSpanProvenance) string {
		return firstNonEmpty(p.Ref, p.StartMessageID+":"+p.EndMessageID)
	}
	for _, p := range existing {
		key := keyFor(p)
		if key != "" {
			index[key] = p
		}
	}
	for _, p := range incoming {
		key := keyFor(p)
		if key == "" {
			continue
		}
		if current, ok := index[key]; ok {
			index[key] = mergeSingleProvenance(current, p)
			continue
		}
		index[key] = p
	}
	out := make([]SourceSpanProvenance, 0, len(index))
	for _, p := range index {
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartMessageID != out[j].StartMessageID {
			return out[i].StartMessageID < out[j].StartMessageID
		}
		return out[i].Ref < out[j].Ref
	})
	return out
}

func mergeSingleProvenance(existing, incoming SourceSpanProvenance) SourceSpanProvenance {
	merged := existing
	merged.Ref = firstNonEmpty(existing.Ref, incoming.Ref)
	merged.StartMessageID = firstNonEmpty(existing.StartMessageID, incoming.StartMessageID)
	merged.EndMessageID = firstNonEmpty(incoming.EndMessageID, existing.EndMessageID)
	merged.SourceMessageIDs = union(existing.SourceMessageIDs, incoming.SourceMessageIDs)
	merged.RunIDs = union(existing.RunIDs, incoming.RunIDs)
	merged.ProposalRefs = union(existing.ProposalRefs, incoming.ProposalRefs)
	merged.FailureRefs = union(existing.FailureRefs, incoming.FailureRefs)
	merged.ArtifactRefs = union(existing.ArtifactRefs, incoming.ArtifactRefs)
	merged.CheckpointRefs = union(existing.CheckpointRefs, incoming.CheckpointRefs)
	return merged
}

func taskSurvivalRecord(task TaskFrame, reason string) TaskSurvivalRecord {
	record := TaskSurvivalRecord{
		TaskID:       task.TaskID,
		RunID:        firstNonEmpty(task.RunID, task.TaskID),
		Reason:       reason,
		ProposalRefs: append([]string(nil), task.ProposalRefs...),
		FailureRefs:  append([]string(nil), task.FailureRefs...),
	}
	if len(task.Provenance) > 0 {
		record.SourceMessageStartID = task.Provenance[0].StartMessageID
		record.SourceMessageEndID = task.Provenance[len(task.Provenance)-1].EndMessageID
	}
	return record
}

func laterTime(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case b.After(a):
		return b
	default:
		return a
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
