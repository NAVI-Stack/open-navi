package compaction

type ScenarioEvidence struct {
	ScenarioName          string                   `json:"scenario_name,omitempty"`
	ScenarioClass         string                   `json:"scenario_class,omitempty"`
	Budget                BudgetSnapshot           `json:"budget"`
	TriggerClass          TriggerClass             `json:"trigger_class,omitempty"`
	TriggerReason         string                   `json:"trigger_reason,omitempty"`
	Selection             CompactionSelection      `json:"selection"`
	SelectedSpan          SourceSpanProvenance     `json:"selected_span"`
	ChatFrame          ChatFrame             `json:"chat_frame"`
	TaskFrames            []TaskFrame              `json:"task_frames,omitempty"`
	RetrievalSpans        []RetrievalSpan          `json:"retrieval_spans,omitempty"`
	RehydrationBeforeTrim RehydrationOutput        `json:"rehydration_before_trim"`
	RehydrationAfterTrim  RehydrationOutput        `json:"rehydration_after_trim"`
	Checkpoint            *CompactionCheckpoint    `json:"checkpoint,omitempty"`
	CheckpointView        CheckpointInspectionView `json:"checkpoint_view"`
	InspectionView        CompactionInspectionView `json:"inspection_view"`
	Error                 string                   `json:"error,omitempty"`
}

func BuildScenarioEvidence(name, class string, input RunInput, memory ChatMemory, output RunOutput, err error) ScenarioEvidence {
	budget := BudgetSnapshot{}
	if output.Checkpoint != nil {
		budget = output.Checkpoint.EstimatorSnapshot
	}
	if budget == (BudgetSnapshot{}) {
		budget = NewBudgetManager(nil, BudgetConfig{}).Snapshot(input.MaxContextTokens, input.ReservedOutputTokens, input.ReservedToolHeadroom, input.Messages)
	}
	liveTail := input.Messages
	if count := output.Inspection.Selection.SelectedCount; count > 0 && count <= len(input.Messages) {
		liveTail = input.Messages[count:]
	}
	beforeTrim := Rehydrator{}.Assemble(RehydrationInput{
		PermanentInstructions: input.PermanentInstructions,
		ChatFrame:          memory.ChatFrame,
		TaskFrames:            memory.TaskFrames,
		ProposalRefs:          input.ProposalRefs,
		FailureRefs:           input.FailureRefs,
		RetrievalSpans:        memory.RetrievalSpans,
		LiveTail:              liveTail,
		NewInput:              input.NewInput,
		Budget:                BudgetSnapshot{UsableTokens: 1 << 30},
		ProtectedRecentWindow: 4,
		MaxTaskFrames:         6,
		MaxSupportSpans:       4,
	})

	evidence := ScenarioEvidence{
		ScenarioName:          name,
		ScenarioClass:         class,
		Budget:                budget,
		TriggerClass:          output.Triggered,
		TriggerReason:         output.Inspection.TriggerReason,
		Selection:             output.Inspection.Selection,
		SelectedSpan:          output.Inspection.SelectedSpan,
		ChatFrame:          memory.ChatFrame,
		TaskFrames:            append([]TaskFrame(nil), memory.TaskFrames...),
		RetrievalSpans:        append([]RetrievalSpan(nil), memory.RetrievalSpans...),
		RehydrationBeforeTrim: beforeTrim,
		RehydrationAfterTrim:  output.Rehydration,
		Checkpoint:            output.Checkpoint,
		InspectionView:        InspectRunOutput(output),
	}
	if output.Checkpoint != nil {
		evidence.CheckpointView = InspectCheckpoint(*output.Checkpoint)
	}
	if err != nil {
		evidence.Error = err.Error()
	}
	return evidence
}
