package command

import "context"

type descriptorContextKey struct{}

// WithDescriptor stores command execution metadata in context so lower layers
// can recover session/run correlation without threading every field manually.
func WithDescriptor(ctx context.Context, desc Descriptor) context.Context {
	return context.WithValue(ctx, descriptorContextKey{}, desc)
}

// DescriptorFromContext returns the active command descriptor when present.
func DescriptorFromContext(ctx context.Context) (Descriptor, bool) {
	desc, ok := ctx.Value(descriptorContextKey{}).(Descriptor)
	return desc, ok
}
