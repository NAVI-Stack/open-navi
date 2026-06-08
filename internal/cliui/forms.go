package cliui

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

// Runner is a form-like value that can be executed with Run().
// All huh.Form values satisfy this interface, as does any custom form wrapper.
type Runner interface {
	Run() error
}

// ClearScreen clears the terminal using ANSI escape codes.
func ClearScreen() {
	fmt.Print("\033[H\033[2J")
}

// Confirm is a wrapper for a simple yes/no confirmation.
func Confirm(title string, description string, value *bool) huh.Field {
	return huh.NewConfirm().
		Title(title).
		Description(description).
		Value(value)
}

// Input is a wrapper for a standard text input.
func Input(title string, description string, placeholder string, value *string) huh.Field {
	return huh.NewInput().
		Title(title).
		Description(description).
		Placeholder(placeholder).
		Value(value)
}

// Secret is a wrapper for a password/secret input.
func Secret(title string, description string, placeholder string, value *string) huh.Field {
	return huh.NewInput().
		Title(title).
		Description(description).
		Placeholder(placeholder).
		EchoMode(huh.EchoModePassword).
		Value(value)
}

// Select is a wrapper for a single-choice selection.
func Select(title string, description string, options []huh.Option[string], value *string) huh.Field {
	return huh.NewSelect[string]().
		Title(title).
		Description(description).
		Options(options...).
		Value(value)
}
