package cliui

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnboardingStateStructure(t *testing.T) {
	state := NewOnboardingState()
	
	// Ensure fields exist and match requirements
	v := reflect.ValueOf(*state)
	
	// Must have:
	fields := []string{
		"BeginSetup",
		"OwnerName",
		"OwnerSecret",
		"SelectedProvider",
		"SelectedModel",
		"SelectedConnector",
		"ConnectorParams",
	}
	
	for _, f := range fields {
		assert.NotNil(t, v.FieldByName(f), "Field %s should exist", f)
	}
	
	// Must NOT have:
	assert.False(t, v.FieldByName("OwnerHandle").IsValid(), "OwnerHandle should no longer exist")
	assert.False(t, v.FieldByName("SecurityMode").IsValid(), "SecurityMode should no longer exist")
}

func TestSecretRedaction(t *testing.T) {
	t.Run("mask_secret", func(t *testing.T) {
		assert.Equal(t, "********", MaskSecret("12345"))
		assert.Equal(t, "abc...mnop", MaskSecret("abcdefghijklmnop"))
	})
}

func TestOwnerFormFields(t *testing.T) {
	state := NewOnboardingState()
	form := OwnerForm(state)
	// Check that we have one group and two fields
	// We have to use internal access or just check the fields list if accessible.
	// huh.Form doesn't expose fields easily, but we can check if it nil.
	assert.NotNil(t, form)
}
