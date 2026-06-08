package intake

import (
	"fmt"
	"time"
)

var knownTrustValues = map[ContentTrust]bool{
	TrustOwner:             true,
	TrustInternalSystem:    true,
	TrustTrustedPlugin:     true,
	TrustExternalUntrusted: true,
}

// AdmitRecord validates and stamps an IntakeRecord for persistence, defaulting
// PrivacyClass to "personal" when the connector supplied none. It is the
// pure-default entry point; the policy-driven entry point is AdmitRecordWithDefaults.
func AdmitRecord(r IntakeRecord) (IntakeRecord, error) {
	return AdmitRecordWithDefaults(r, PrivacyPersonal)
}

// AdmitRecordWithDefaults validates and stamps an IntakeRecord for persistence:
//   - rejects records missing ConnectorID, SourceID, or an unknown Trust value
//   - sources PrivacyClass from the connector's sync policy (defaultPrivacy) when
//     the record itself carries none (CIP §9: privacy class assigned at Admit,
//     now policy-driven per CIP P5 §7)
//   - defaults RawMIME to "text/plain" if empty
//   - sets FetchedAt to now if zero
//   - stamps Provenance.ConnectorID from ConnectorID if not already set
//
// Returns the stamped record. No I/O — pure function.
func AdmitRecordWithDefaults(r IntakeRecord, defaultPrivacy PrivacyClass) (IntakeRecord, error) {
	if r.ConnectorID == "" {
		return r, fmt.Errorf("intake: admit: missing ConnectorID")
	}
	if r.SourceID == "" {
		return r, fmt.Errorf("intake: admit: missing SourceID")
	}
	if !knownTrustValues[r.Trust] {
		return r, fmt.Errorf("intake: admit: unknown trust value %q", r.Trust)
	}
	if r.PrivacyClass == "" {
		if defaultPrivacy == "" {
			defaultPrivacy = PrivacyPersonal
		}
		r.PrivacyClass = defaultPrivacy
	}
	if r.RawMIME == "" {
		r.RawMIME = "text/plain"
	}
	if r.FetchedAt.IsZero() {
		r.FetchedAt = time.Now().UTC()
	}
	if r.Provenance.ConnectorID == "" {
		r.Provenance.ConnectorID = r.ConnectorID
	}
	return r, nil
}
