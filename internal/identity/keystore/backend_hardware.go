package keystore

import (
	"context"
	"fmt"
)

// HardwareStore is a placeholder for future TPM/HSM integrations.
type HardwareStore struct{}

func (HardwareStore) Save(_ context.Context, _ string, _ []byte) error {
	return fmt.Errorf("%w: hardware", ErrNotImplemented)
}

func (HardwareStore) Load(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("%w: hardware", ErrNotImplemented)
}

func (HardwareStore) Exists(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("%w: hardware", ErrNotImplemented)
}
