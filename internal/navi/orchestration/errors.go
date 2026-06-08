package orchestration

import (
	"errors"
	"fmt"
)

var (
	ErrMissingContextAssembler    = errors.New("orchestration: missing context assembler")
	ErrMissingInstructionCompiler = errors.New("orchestration: missing instruction compiler")
	ErrMissingModelAdapter        = errors.New("orchestration: missing model adapter")
)

type StageError struct {
	Stage string
	Err   error
}

func (e *StageError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Stage == "" && e.Err == nil {
		return "orchestration stage error"
	}
	if e.Stage == "" {
		return e.Err.Error()
	}
	if e.Err == nil {
		return fmt.Sprintf("orchestration stage %q failed", e.Stage)
	}
	return fmt.Sprintf("orchestration stage %q: %v", e.Stage, e.Err)
}

func (e *StageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
