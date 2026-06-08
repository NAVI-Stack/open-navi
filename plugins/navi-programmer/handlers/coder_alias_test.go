package handlers

import (
	"testing"

	coreskill "github.com/open-navi/navi/internal/navi/skill"
)

// TestRegisterProgrammerHandlersRegistersCoderAlias proves the run-validation
// internal handlers are reachable under both the legacy id and the Coder-facing
// alias (migration ticket NEW-A), so a direct GetInternalHandler with the coder
// id resolves to the same implementation.
func TestRegisterProgrammerHandlersRegistersCoderAlias(t *testing.T) {
	RegisterProgrammerHandlers(RunValidationSkillID, ProgrammerHandlerConfig{})

	ids := []string{
		"navi-programmer.run-validation", // legacy still works
		"navi-coder.run-validation",      // coder alias resolves
	}
	for _, id := range ids {
		for _, iface := range []string{"run_command", "record_not_run"} {
			if _, ok := coreskill.GetInternalHandler(id, iface); !ok {
				t.Errorf("GetInternalHandler(%q, %q) not registered", id, iface)
			}
		}
	}
}
