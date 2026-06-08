package compaction

import (
	"regexp"
	"strings"
)

type SegmentSelector struct {
	DefaultProtectedRecentWindow int
	MinimumProtectedRecentWindow int
}

type messageSignal struct {
	toolStart         bool
	toolEnd           bool
	proposalStart     bool
	proposalEnd       bool
	failureOpen       bool
	recoveryActive    bool
	recoveryResolved  bool
	artifactMutation  bool
	artifactSettled   bool
	bundleKey         string
	artifactBundleKey string
	reason            BoundaryReason
}

type scanState struct {
	openToolCalls                map[string]int
	openProposals                map[string]int
	openRecoveries               map[string]int
	artifactMutationOpen         map[string]bool
	hasToolResultWithoutCall     bool
	hasResolutionWithoutProposal bool
	hasRecoveryCloseWithoutOpen  bool
}

type selectorContext struct {
	runs map[string]RunSnapshot
}

func (s SegmentSelector) Select(input SelectorInput) (SelectionResult, bool) {
	messages := input.Messages
	if len(messages) == 0 {
		return SelectionResult{}, false
	}
	window := s.DefaultProtectedRecentWindow
	if window <= 0 {
		window = 10
	}
	minWindow := s.MinimumProtectedRecentWindow
	if minWindow <= 0 {
		minWindow = 4
	}
	if input.Trigger == TriggerEmergency && window > minWindow {
		window = minWindow
	}
	trace := CompactionSelection{
		ProtectedRecentWindow: window,
		CandidateStartID:      messages[0].ID,
	}
	limit := len(messages) - window
	if limit <= 0 {
		return SelectionResult{Trace: trace}, false
	}
	ctx := buildSelectorContext(input.RuntimeSnapshots)
	protectedFrom := protectedLatestArtifactIndex(messages, ctx)
	if protectedFrom >= 0 && protectedFrom < limit {
		limit = protectedFrom
		trace.StopReason = &BoundaryReason{
			Kind:      "artifact_mutation",
			MessageID: messages[protectedFrom].ID,
			RunID:     strings.TrimSpace(messages[protectedFrom].RunID),
			RefID:     firstArtifactKey(messages[protectedFrom], ctx),
			Detail:    "latest artifact mutation bundle remains protected",
		}
	}
	end := -1
	for i := 0; i < limit; i++ {
		if reason, blocked := boundaryCrosses(messages, i, ctx); blocked {
			trace.StopReason = &reason
			break
		}
		end = i
	}
	if end < 1 {
		return SelectionResult{Trace: trace}, false
	}
	trace.CandidateEndID = messages[end].ID
	trace.SelectedCount = end + 1
	return SelectionResult{
		StartIndex: 0,
		EndIndex:   end,
		Messages:   append([]Message(nil), messages[:end+1]...),
		Trace:      trace,
	}, true
}

func buildSelectorContext(runs []RunSnapshot) selectorContext {
	ctx := selectorContext{runs: make(map[string]RunSnapshot, len(runs))}
	for _, run := range runs {
		if key := strings.TrimSpace(run.RunID); key != "" {
			ctx.runs[key] = run
		}
	}
	return ctx
}

func boundaryCrosses(messages []Message, cut int, ctx selectorContext) (BoundaryReason, bool) {
	left := boundaryState(messages[:cut+1], ctx)
	right := boundaryState(messages[cut+1:], ctx)
	if len(left.openToolCalls) > 0 {
		return BoundaryReason{Kind: "tool_bundle", MessageID: messages[cut].ID, Detail: "tool call/result bundle remains open"}, true
	}
	if len(left.openProposals) > 0 {
		return BoundaryReason{Kind: "proposal_bundle", MessageID: messages[cut].ID, Detail: "proposal bundle remains open"}, true
	}
	if len(left.openRecoveries) > 0 {
		return BoundaryReason{Kind: "recovery_bundle", MessageID: messages[cut].ID, Detail: "failure/recovery bundle remains open"}, true
	}
	if len(left.artifactMutationOpen) > 0 {
		return BoundaryReason{Kind: "artifact_mutation", MessageID: messages[cut].ID, Detail: "artifact mutation bundle remains open"}, true
	}
	if right.hasResolutionWithoutProposal {
		return BoundaryReason{Kind: "proposal_bundle", MessageID: messages[cut+1].ID, Detail: "would strand proposal resolution without open proposal"}, true
	}
	if right.hasToolResultWithoutCall {
		return BoundaryReason{Kind: "tool_bundle", MessageID: messages[cut+1].ID, Detail: "would strand tool result without tool call"}, true
	}
	if right.hasRecoveryCloseWithoutOpen {
		return BoundaryReason{Kind: "recovery_bundle", MessageID: messages[cut+1].ID, Detail: "would strand recovery closure without failure context"}, true
	}
	return BoundaryReason{}, false
}

func boundaryState(messages []Message, ctx selectorContext) scanState {
	state := scanState{
		openToolCalls:        map[string]int{},
		openProposals:        map[string]int{},
		openRecoveries:       map[string]int{},
		artifactMutationOpen: map[string]bool{},
	}
	for _, msg := range messages {
		signal := analyzeMessage(msg, ctx)
		switch {
		case signal.toolStart:
			state.openToolCalls[signal.bundleKey]++
		case signal.toolEnd:
			if state.openToolCalls[signal.bundleKey] > 0 {
				state.openToolCalls[signal.bundleKey]--
				if state.openToolCalls[signal.bundleKey] == 0 {
					delete(state.openToolCalls, signal.bundleKey)
				}
			} else {
				state.hasToolResultWithoutCall = true
			}
		}
		switch {
		case signal.proposalStart:
			state.openProposals[signal.bundleKey]++
		case signal.proposalEnd:
			if state.openProposals[signal.bundleKey] > 0 {
				state.openProposals[signal.bundleKey]--
				if state.openProposals[signal.bundleKey] == 0 {
					delete(state.openProposals, signal.bundleKey)
				}
			} else {
				state.hasResolutionWithoutProposal = true
			}
		}
		switch {
		case signal.failureOpen, signal.recoveryActive:
			state.openRecoveries[signal.bundleKey]++
		case signal.recoveryResolved:
			if state.openRecoveries[signal.bundleKey] > 0 {
				state.openRecoveries[signal.bundleKey]--
				if state.openRecoveries[signal.bundleKey] == 0 {
					delete(state.openRecoveries, signal.bundleKey)
				}
			} else {
				state.hasRecoveryCloseWithoutOpen = true
			}
		}
		if signal.artifactMutation {
			state.artifactMutationOpen[signal.artifactBundleKey] = true
		}
		if signal.artifactSettled {
			delete(state.artifactMutationOpen, signal.artifactBundleKey)
		}
	}
	return state
}

func protectedLatestArtifactIndex(messages []Message, ctx selectorContext) int {
	latest := -1
	latestBundle := ""
	for i := len(messages) - 1; i >= 0; i-- {
		signal := analyzeMessage(messages[i], ctx)
		if latest == -1 && signal.artifactMutation {
			latest = i
			latestBundle = signal.artifactBundleKey
			continue
		}
		if latest == -1 {
			continue
		}
		if latestBundle == "" || signal.artifactBundleKey != latestBundle {
			break
		}
		latest = i
	}
	return latest
}

func analyzeMessage(msg Message, ctx selectorContext) messageSignal {
	kind := normalizeKind(msg.MessageKind)
	content := normalizeContent(msg.Content)
	bundleKey := firstSignalKey(msg, ctx)
	artifactKey := firstArtifactKey(msg, ctx)
	signal := messageSignal{
		bundleKey:         bundleKey,
		artifactBundleKey: artifactKey,
	}
	switch kind {
	case "tool_call":
		signal.toolStart = true
	case "tool_result":
		signal.toolEnd = true
	case "proposal_open":
		signal.proposalStart = true
	case "proposal_resolution":
		signal.proposalEnd = true
	case "failure_open":
		signal.failureOpen = true
	case "recovery_active":
		signal.recoveryActive = true
	case "recovery_resolved":
		signal.recoveryResolved = true
	case "artifact_mutation":
		signal.artifactMutation = true
	case "artifact_mutation_settled":
		signal.artifactSettled = true
	}
	if run, ok := ctx.runs[strings.TrimSpace(msg.RunID)]; ok {
		if run.PendingToolCall && !signal.toolEnd {
			signal.toolStart = true
		}
		if run.PendingProposalID != "" || run.BlockedOnProposalID != "" {
			if !signal.proposalEnd {
				signal.proposalStart = true
			}
		}
		if len(run.ArtifactIDs) > 0 || strings.TrimSpace(run.MainArtifactID) != "" || len(run.CheckpointArtifactIDs) > 0 || strings.TrimSpace(run.CheckpointMainArtifact) != "" {
			if !signal.artifactSettled {
				signal.artifactMutation = true
			}
		}
	}
	if !signal.toolStart && toolStartPattern.MatchString(content) {
		signal.toolStart = true
	}
	if !signal.toolEnd && toolEndPattern.MatchString(content) {
		signal.toolEnd = true
	}
	if !signal.proposalStart && proposalStartPattern.MatchString(content) {
		signal.proposalStart = true
	}
	if !signal.proposalEnd && proposalResolutionPattern.MatchString(content) {
		signal.proposalEnd = true
	}
	if !signal.failureOpen && failureOpenPattern.MatchString(content) {
		signal.failureOpen = true
	}
	if !signal.recoveryActive && recoveryActivePattern.MatchString(content) {
		signal.recoveryActive = true
	}
	if !signal.recoveryResolved && recoveryResolvedPattern.MatchString(content) {
		signal.recoveryResolved = true
	}
	if !signal.artifactMutation && artifactMutationPattern.MatchString(content) {
		signal.artifactMutation = true
	}
	if !signal.artifactSettled && artifactSettledPattern.MatchString(content) {
		signal.artifactSettled = true
	}
	return signal
}

func normalizeKind(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	switch {
	case strings.HasPrefix(k, "tool_call"):
		return "tool_call"
	case strings.HasPrefix(k, "tool_result"):
		return "tool_result"
	case strings.HasPrefix(k, "proposal"):
		if strings.Contains(k, "resolution") || strings.Contains(k, "approved") || strings.Contains(k, "denied") {
			return "proposal_resolution"
		}
		return "proposal_open"
	case strings.HasPrefix(k, "failure_open"), strings.HasPrefix(k, "failure"):
		return "failure_open"
	case strings.HasPrefix(k, "recovery_active"):
		return "recovery_active"
	case strings.HasPrefix(k, "recovery_resolved"), strings.HasPrefix(k, "recovery_closed"):
		return "recovery_resolved"
	case strings.HasPrefix(k, "artifact_mutation_latest"), strings.HasPrefix(k, "artifact_mutation"):
		return "artifact_mutation"
	case strings.HasPrefix(k, "artifact_settled"), strings.HasPrefix(k, "artifact_mutation_settled"):
		return "artifact_mutation_settled"
	default:
		return k
	}
}
func normalizeContent(content string) string {
	return strings.ToLower(strings.TrimSpace(strings.Join(strings.Fields(content), " ")))
}

func firstSignalKey(msg Message, ctx selectorContext) string {
	if runID := strings.TrimSpace(msg.RunID); runID != "" {
		return runID
	}
	for _, candidate := range []string{
		extractTaggedID(msg.Content, "proposal"),
		extractTaggedID(msg.Content, "failure"),
		extractTaggedID(msg.Content, "recovery"),
		extractTaggedID(msg.Content, "tool"),
		extractTaggedID(msg.Content, "call"),
	} {
		if candidate != "" {
			return candidate
		}
	}
	return "global"
}

func firstArtifactKey(msg Message, ctx selectorContext) string {
	if run, ok := ctx.runs[strings.TrimSpace(msg.RunID)]; ok {
		for _, candidate := range append([]string{strings.TrimSpace(run.MainArtifactID), strings.TrimSpace(run.CheckpointMainArtifact)}, append(append([]string(nil), run.ArtifactIDs...), run.CheckpointArtifactIDs...)...) {
			if trimmed := strings.TrimSpace(candidate); trimmed != "" {
				return strings.ToLower(trimmed)
			}
		}
	}
	for _, candidate := range []string{
		extractTaggedID(msg.Content, "artifact"),
		extractTaggedID(msg.Content, "file"),
		extractPathHint(msg.Content),
		strings.TrimSpace(msg.RunID),
	} {
		if candidate != "" {
			return strings.ToLower(strings.TrimSpace(candidate))
		}
	}
	return "artifact-global"
}

func extractTaggedID(content, label string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\[` + regexp.QuoteMeta(label) + `:([a-z0-9._:/-]+)\]`),
		regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(label) + `(?:_id| ref| id)?[:=]\s*([a-z0-9._:/-]+)`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(content)
		if len(match) == 2 {
			return strings.ToLower(strings.TrimSpace(match[1]))
		}
	}
	return ""
}

func extractPathHint(content string) string {
	match := pathHintPattern.FindString(strings.TrimSpace(content))
	return strings.ToLower(strings.TrimSpace(match))
}

var (
	toolStartPattern          = regexp.MustCompile(`(?i)\b(tool call|calling tool|invoking tool|running tool|executing tool)\b`)
	toolEndPattern            = regexp.MustCompile(`(?i)\b(tool result|tool output|command output|result:)\b`)
	proposalStartPattern      = regexp.MustCompile(`(?i)\b(proposal|approval required|proposed action|needs approval)\b`)
	proposalResolutionPattern = regexp.MustCompile(`(?i)\b(approve|approved|deny|denied|decline|declined|reject|rejected)\b`)
	failureOpenPattern        = regexp.MustCompile(`(?i)\b(failure|failed|error|crashed|exception)\b`)
	recoveryActivePattern     = regexp.MustCompile(`(?i)\b(recovery|retrying|attempting fix|working around|investigating)\b`)
	recoveryResolvedPattern   = regexp.MustCompile(`(?i)\b(recovery resolved|resolved|fixed|recovered|succeeded after retry)\b`)
	artifactMutationPattern   = regexp.MustCompile(`(?i)\b(created|updated|modified|wrote|patched|edited)\b.*\b(file|artifact|document|report|plan)\b`)
	artifactSettledPattern    = regexp.MustCompile(`(?i)\b(saved|verified|looks good|artifact settled|change confirmed)\b`)
	pathHintPattern           = regexp.MustCompile(`(?i)([a-z]:\\[^ \n\t]+|/[^ \n\t]+|[a-z0-9._-]+\.(go|md|txt|json|yaml|yml|sql|html|css|js|ts))`)
)
