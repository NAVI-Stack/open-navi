package store

import "errors"

// ValidationError identifies a user-fixable persistence validation failure.
type ValidationError struct {
	Entity  string
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "store: validation failed"
	}
	if e.Field == "" {
		return "store: invalid " + e.Entity + ": " + e.Message
	}
	return "store: invalid " + e.Entity + "." + e.Field + ": " + e.Message
}

// IsValidationError reports whether err is a ValidationError.
func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}
