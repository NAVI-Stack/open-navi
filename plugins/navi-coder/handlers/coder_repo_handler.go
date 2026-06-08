package handlers

import (
	"context"
	"fmt"
	"strings"

	coreskill "github.com/ceoai/navi/internal/navi/skill"
	naviprogrammer "github.com/ceoai/navi/plugins/navi-programmer/handlers"
)

const CoderRepoSkillID = "navi.coder.repo"

// RegisterCoderRepoHandlers registers governed internal handlers for navi.coder.repo.
func RegisterCoderRepoHandlers(skillID string) {
	if strings.TrimSpace(skillID) == "" {
		skillID = CoderRepoSkillID
	}
	coreskill.RegisterInternalHandler(skillID, "run_tests", runTests)
}

func runTests(ctx context.Context, entry *coreskill.SkillEntry, iface *coreskill.Interface, args map[string]any) (any, error) {
	if args == nil {
		args = map[string]any{}
	}
	command, ok := args["command"]
	if !ok || command == nil {
		command = args["test_command"]
	}
	if command == nil {
		return nil, fmt.Errorf("command or test_command is required")
	}
	validationArgs := make(map[string]any, len(args)+2)
	for key, value := range args {
		validationArgs[key] = value
	}
	validationArgs["command"] = command
	if strings.TrimSpace(fmt.Sprint(validationArgs["validation_kind"])) == "" {
		validationArgs["validation_kind"] = "test"
	}
	handler, ok := coreskill.GetInternalHandler(naviprogrammer.RunValidationSkillID, "run_command")
	if !ok {
		return nil, fmt.Errorf("validation handler is not registered")
	}
	return handler(ctx, entry, iface, validationArgs)
}
