package skill

import (
	"fmt"
)

// TrustLevel defines the reliability of data.
type TrustLevel string

const (
	TrustTrusted   TrustLevel = "trusted"   // internal data, verified user input
	TrustUntrusted TrustLevel = "untrusted" // tool outputs, web content, third-party files
)

// Provenance tracks the origin and trust level of data.
type Provenance struct {
	Level  TrustLevel `json:"level"`
	Source string     `json:"source"` // e.g., "tool:google_search", "file:notes.txt"
}

// DataEnvelope wraps content with its provenance.
type DataEnvelope struct {
	Content    string     `json:"content"`
	Provenance Provenance `json:"provenance"`
}

// NewUntrustedEnvelope creates an envelope for data from an untrusted source.
func NewUntrustedEnvelope(source string, content string) DataEnvelope {
	return DataEnvelope{
		Content: content,
		Provenance: Provenance{
			Level:  TrustUntrusted,
			Source: source,
		},
	}
}

// SanitizeResult ensures tool outputs are treated as untrusted data.
// In a full implementation, this might strip markdown instructions or
// use a separate "content-only" channel in the prompt.
func SanitizeResult(source string, raw string) DataEnvelope {
	// Simple sanitization: just tag it.
	return NewUntrustedEnvelope(source, raw)
}

// FormatForPrompt wraps content with XML tags to signal trust boundaries to the LLM.
func (de DataEnvelope) FormatForPrompt() string {
	if de.Provenance.Level == TrustUntrusted {
		return fmt.Sprintf("<untrusted_data source=%q>\n%s\n</untrusted_data>", de.Provenance.Source, de.Content)
	}
	return de.Content
}
