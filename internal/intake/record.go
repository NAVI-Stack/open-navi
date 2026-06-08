// Package intake implements the Context Intake Pipeline Phase 1 (CIP P1):
// admission, deduplication, and persistence of IntakeRecords emitted by
// connectors onto the NAVI_REFINERY NATS stream.
//
// Types (IntakeRecord, ContentTrust, PrivacyClass, IntakeProvenance) live in
// internal/schema to avoid import cycles with internal/store.
package intake

import "github.com/open-navi/navi/internal/schema"

// Type aliases so callers can use intake.IntakeRecord etc. without importing schema.
type IntakeRecord = schema.IntakeRecord
type ContentTrust = schema.ContentTrust
type PrivacyClass = schema.PrivacyClass
type Provenance = schema.IntakeProvenance

// Trust constants.
const (
	TrustOwner             = schema.ContentTrustOwner
	TrustInternalSystem    = schema.ContentTrustInternalSystem
	TrustTrustedPlugin     = schema.ContentTrustTrustedPlugin
	TrustExternalUntrusted = schema.ContentTrustExternalUntrusted
)

// Privacy class constants.
const (
	PrivacyPublic    = schema.PrivacyClassPublic
	PrivacyPersonal  = schema.PrivacyClassPersonal
	PrivacySensitive = schema.PrivacyClassSensitive
	PrivacySecret    = schema.PrivacyClassSecret
)
