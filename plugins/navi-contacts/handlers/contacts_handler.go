package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	coreskill "github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

type SkillEntry = coreskill.SkillEntry
type Interface = coreskill.Interface

var RegisterInternalHandler = coreskill.RegisterInternalHandler

// RegisterContactsHandler registers all navi-contacts skill interfaces against the given skill ID.
// The skillID must match the skill_id field in SKILL.yaml (e.g. "navi-contacts").
func RegisterContactsHandler(skillID string, db *sql.DB) {
	if db == nil {
		return
	}
	registerCreateContact(skillID, db)
	registerGetContact(skillID, db)
	registerListContacts(skillID, db)
	registerUpdateContact(skillID, db)
	registerDeleteContact(skillID, db)
	registerSearchContacts(skillID, db)
	registerImportVCF(skillID, db)
	registerExportVCF(skillID, db)
	registerShareContact(skillID, db)
	registerLinkIdentity(skillID, db)
	registerRecordInteraction(skillID, db)
}

// ── create_contact ────────────────────────────────────────────────────────────

func registerCreateContact(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "create_contact", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		name := strings.TrimSpace(stringArg(args, "name"))
		if name == "" {
			return nil, fmt.Errorf("name is required")
		}
		kind := stringArg(args, "kind")
		if kind == "" {
			kind = "person"
		}
		trustLevel := stringArg(args, "trust_level")
		if trustLevel == "" {
			trustLevel = schema.ContactTrustLevelUnknown
		}
		meta := buildMetadataFromArgs(args)
		meta.Source = "manual"

		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		c := schema.Contact{
			ID:         uuid.New().String(),
			Name:       name,
			Kind:       kind,
			OwnerType:  schema.ContactOwnerTypeNavi,
			TrustLevel: trustLevel,
			Metadata:   string(metaJSON),
		}
		if err := store.SaveContact(ctx, db, c); err != nil {
			return nil, err
		}
		saved, err := store.GetContact(ctx, db, c.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"id":          saved.ID,
			"name":        saved.Name,
			"kind":        saved.Kind,
			"owner_type":  saved.OwnerType,
			"trust_level": saved.TrustLevel,
			"created_at":  saved.CreatedAt.Format(time.RFC3339),
		}, nil
	})
}

// ── get_contact ───────────────────────────────────────────────────────────────

func registerGetContact(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "get_contact", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		id := strings.TrimSpace(stringArg(args, "id"))
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		c, err := store.GetContact(ctx, db, id)
		if err != nil {
			return nil, err
		}
		var meta ContactMetadata
		if c.Metadata != "" && c.Metadata != "{}" {
			_ = json.Unmarshal([]byte(c.Metadata), &meta)
		}
		return map[string]any{
			"id":          c.ID,
			"name":        c.Name,
			"kind":        c.Kind,
			"owner_type":  c.OwnerType,
			"trust_level": c.TrustLevel,
			"metadata":    meta,
			"created_at":  c.CreatedAt.Format(time.RFC3339),
			"updated_at":  c.UpdatedAt.Format(time.RFC3339),
		}, nil
	})
}

// ── list_contacts ─────────────────────────────────────────────────────────────

func registerListContacts(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "list_contacts", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		ownerType := stringArg(args, "owner_type")
		if ownerType == "all" {
			ownerType = ""
		} else if ownerType == "" {
			ownerType = schema.ContactOwnerTypeNavi
		}
		limit := intArg(args, "limit", 50)
		if limit > 200 {
			limit = 200
		}
		contacts, err := store.ListContacts(ctx, db, store.ListContactsFilter{
			OwnerType:  ownerType,
			Kind:       stringArg(args, "kind"),
			TrustLevel: stringArg(args, "trust_level"),
			NamePrefix: stringArg(args, "name_prefix"),
			Limit:      limit,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"contacts": summarizeContacts(contacts),
			"total":    len(contacts),
		}, nil
	})
}

// ── update_contact ────────────────────────────────────────────────────────────

func registerUpdateContact(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "update_contact", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		id := strings.TrimSpace(stringArg(args, "id"))
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		c, err := store.GetContact(ctx, db, id)
		if err != nil {
			return nil, err
		}

		// Parse existing metadata
		var meta ContactMetadata
		if c.Metadata != "" && c.Metadata != "{}" {
			_ = json.Unmarshal([]byte(c.Metadata), &meta)
		}

		// Update top-level fields if present in args
		if v, ok := args["name"]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				c.Name = strings.TrimSpace(s)
			}
		}
		if v, ok := args["kind"]; ok {
			if s, ok := v.(string); ok && s != "" {
				c.Kind = s
			}
		}
		if v, ok := args["trust_level"]; ok {
			if s, ok := v.(string); ok && s != "" {
				c.TrustLevel = s
			}
		}

		// Apply metadata field updates selectively
		if _, ok := args["first_name"]; ok {
			meta.FirstName = stringArg(args, "first_name")
		}
		if _, ok := args["last_name"]; ok {
			meta.LastName = stringArg(args, "last_name")
		}
		if _, ok := args["emails"]; ok {
			meta.Emails = parseLabeledValues(args["emails"])
		}
		if _, ok := args["phones"]; ok {
			meta.Phones = parseLabeledValues(args["phones"])
		}
		if _, ok := args["addresses"]; ok {
			meta.Addresses = parseAddresses(args["addresses"])
		}
		if _, ok := args["organization"]; ok {
			meta.Organization = parseOrganization(args["organization"])
		}
		if _, ok := args["birthday"]; ok {
			meta.Birthday = stringArg(args, "birthday")
		}
		if _, ok := args["notes"]; ok {
			meta.Notes = stringArg(args, "notes")
		}
		if _, ok := args["tags"]; ok {
			meta.Tags = parseStringSlice(args["tags"])
		}
		if _, ok := args["photo_url"]; ok {
			meta.PhotoURL = stringArg(args, "photo_url")
		}

		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		c.Metadata = string(metaJSON)

		if err := store.SaveContact(ctx, db, c); err != nil {
			return nil, err
		}
		saved, err := store.GetContact(ctx, db, c.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"id":          saved.ID,
			"name":        saved.Name,
			"trust_level": saved.TrustLevel,
			"updated_at":  saved.UpdatedAt.Format(time.RFC3339),
		}, nil
	})
}

// ── delete_contact ────────────────────────────────────────────────────────────

func registerDeleteContact(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "delete_contact", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		id := strings.TrimSpace(stringArg(args, "id"))
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		if err := store.DeleteContact(ctx, db, id); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "id": id}, nil
	})
}

// ── search_contacts ───────────────────────────────────────────────────────────

func registerSearchContacts(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "search_contacts", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		query := strings.TrimSpace(stringArg(args, "query"))
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}
		ownerType := stringArg(args, "owner_type")
		if ownerType == "all" {
			ownerType = ""
		}
		results, err := store.SearchContacts(ctx, db, store.SearchContactsFilter{
			Query:     query,
			OwnerType: ownerType,
			Limit:     intArg(args, "limit", 20),
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"results": summarizeContacts(results),
			"total":   len(results),
		}, nil
	})
}

// ── import_vcf ────────────────────────────────────────────────────────────────

func registerImportVCF(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "import_vcf", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		vcf := stringArg(args, "vcf")
		if strings.TrimSpace(vcf) == "" {
			return nil, fmt.Errorf("vcf is required")
		}
		ownerType := stringArg(args, "owner_type")
		if ownerType == "" {
			ownerType = schema.ContactOwnerTypeNavi
		}
		if ownerType != schema.ContactOwnerTypeNavi && ownerType != schema.ContactOwnerTypeOwner {
			return nil, fmt.Errorf("owner_type must be 'navi' or 'owner'")
		}
		mergeDuplicates := boolArg(args, "merge_duplicates", false)

		parsed, parseErrors := ParseVCF(vcf)

		var imported, skipped, errCount int
		var ids []string

		for _, card := range parsed {
			if card.Name == "" {
				errCount++
				continue
			}

			metaJSON, err := json.Marshal(card.Metadata)
			if err != nil {
				errCount++
				continue
			}

			if mergeDuplicates {
				// Search for existing contact with same name
				existing, searchErr := store.SearchContacts(ctx, db, store.SearchContactsFilter{
					Query:     card.Name,
					OwnerType: ownerType,
					Limit:     1,
				})
				if searchErr == nil && len(existing) > 0 && strings.EqualFold(existing[0].Name, card.Name) {
					existing[0].Metadata = string(metaJSON)
					if saveErr := store.SaveContact(ctx, db, existing[0]); saveErr == nil {
						ids = append(ids, existing[0].ID)
						imported++
					} else {
						errCount++
					}
					continue
				}
			}

			c := schema.Contact{
				ID:        uuid.New().String(),
				Name:      card.Name,
				Kind:      card.Kind,
				OwnerType: ownerType,
				Metadata:  string(metaJSON),
			}
			if err := store.SaveContact(ctx, db, c); err != nil {
				errCount++
				continue
			}
			ids = append(ids, c.ID)
			imported++
		}

		skipped = len(parseErrors)
		_ = skipped

		return map[string]any{
			"imported": imported,
			"skipped":  len(parseErrors),
			"errors":   errCount,
			"ids":      ids,
		}, nil
	})
}

// ── export_vcf ────────────────────────────────────────────────────────────────

func registerExportVCF(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "export_vcf", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		ownerType := stringArg(args, "owner_type")
		if ownerType == "all" {
			ownerType = ""
		} else if ownerType == "" {
			ownerType = schema.ContactOwnerTypeNavi
		}

		var contacts []schema.Contact

		// If specific IDs are provided, fetch those individually
		if rawIDs, ok := args["ids"]; ok {
			if idSlice, ok := rawIDs.([]any); ok && len(idSlice) > 0 {
				for _, rawID := range idSlice {
					if idStr, ok := rawID.(string); ok && idStr != "" {
						c, err := store.GetContact(ctx, db, idStr)
						if err == nil {
							contacts = append(contacts, c)
						}
					}
				}
			}
		}

		// Otherwise list by filter
		if len(contacts) == 0 {
			var err error
			contacts, err = store.ListContacts(ctx, db, store.ListContactsFilter{
				OwnerType: ownerType,
				Kind:      stringArg(args, "kind"),
				Limit:     1000,
			})
			if err != nil {
				return nil, err
			}
		}

		var sb strings.Builder
		for _, c := range contacts {
			var meta ContactMetadata
			if c.Metadata != "" && c.Metadata != "{}" {
				_ = json.Unmarshal([]byte(c.Metadata), &meta)
			}
			sb.WriteString(ContactToVCF(c.Name, meta))
		}

		return map[string]any{
			"vcf":   sb.String(),
			"count": len(contacts),
		}, nil
	})
}

// ── share_contact ─────────────────────────────────────────────────────────────

func registerShareContact(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "share_contact", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		id := strings.TrimSpace(stringArg(args, "id"))
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		c, err := store.GetContact(ctx, db, id)
		if err != nil {
			return nil, err
		}
		var meta ContactMetadata
		if c.Metadata != "" && c.Metadata != "{}" {
			_ = json.Unmarshal([]byte(c.Metadata), &meta)
		}
		return map[string]any{
			"vcf":  ContactToVCF(c.Name, meta),
			"name": c.Name,
		}, nil
	})
}

// ── link_identity ─────────────────────────────────────────────────────────────

func registerLinkIdentity(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "link_identity", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		id := strings.TrimSpace(stringArg(args, "contact_id"))
		if id == "" {
			return nil, fmt.Errorf("contact_id is required")
		}
		identifierType := strings.TrimSpace(stringArg(args, "identifier_type"))
		identifierValue := strings.TrimSpace(stringArg(args, "identifier_value"))
		if identifierType == "" {
			return nil, fmt.Errorf("identifier_type is required (email, phone, or navi_id)")
		}
		if identifierValue == "" {
			return nil, fmt.Errorf("identifier_value is required")
		}

		c, err := store.GetContact(ctx, db, id)
		if err != nil {
			return nil, err
		}
		var meta ContactMetadata
		if c.Metadata != "" && c.Metadata != "{}" {
			_ = json.Unmarshal([]byte(c.Metadata), &meta)
		}

		switch identifierType {
		case "email":
			// Deduplicate
			for _, e := range meta.Emails {
				if strings.EqualFold(e.Value, identifierValue) {
					return map[string]any{"success": false, "message": "email already linked"}, nil
				}
			}
			exists, err := identifierExistsOnOtherContact(ctx, db, id, identifierType, identifierValue)
			if err != nil {
				return nil, err
			}
			if exists {
				return map[string]any{"success": false, "message": "email already linked to another contact"}, nil
			}
			meta.Emails = append(meta.Emails, LabeledValue{Value: identifierValue})
		case "phone":
			for _, p := range meta.Phones {
				if p.Value == identifierValue {
					return map[string]any{"success": false, "message": "phone already linked"}, nil
				}
			}
			exists, err := identifierExistsOnOtherContact(ctx, db, id, identifierType, identifierValue)
			if err != nil {
				return nil, err
			}
			if exists {
				return map[string]any{"success": false, "message": "phone already linked to another contact"}, nil
			}
			meta.Phones = append(meta.Phones, LabeledValue{Value: identifierValue})
		case "navi_id":
			if meta.NaviID == identifierValue {
				return map[string]any{"success": false, "message": "navi_id already linked"}, nil
			}
			exists, err := identifierExistsOnOtherContact(ctx, db, id, identifierType, identifierValue)
			if err != nil {
				return nil, err
			}
			if exists {
				return map[string]any{"success": false, "message": "navi_id already linked to another contact"}, nil
			}
			meta.NaviID = identifierValue
		default:
			return nil, fmt.Errorf("identifier_type must be email, phone, or navi_id")
		}

		meta = appendInteraction(meta, "identity_linked", identifierType+":"+identifierValue)

		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		c.Metadata = string(metaJSON)
		if err := store.SaveContact(ctx, db, c); err != nil {
			return nil, err
		}
		return map[string]any{"success": true, "message": identifierType + " linked"}, nil
	})
}

func identifierExistsOnOtherContact(ctx context.Context, db *sql.DB, contactID, identifierType, identifierValue string) (bool, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT metadata
		FROM contacts
		WHERE id <> ? AND metadata LIKE ?
	`, contactID, "%"+identifierValue+"%")
	if err != nil {
		return false, fmt.Errorf("search linked identifiers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, fmt.Errorf("scan linked identifier metadata: %w", err)
		}
		var meta ContactMetadata
		if raw != "" && raw != "{}" {
			_ = json.Unmarshal([]byte(raw), &meta)
		}
		switch identifierType {
		case "email":
			for _, email := range meta.Emails {
				if strings.EqualFold(email.Value, identifierValue) {
					return true, nil
				}
			}
		case "phone":
			for _, phone := range meta.Phones {
				if phone.Value == identifierValue {
					return true, nil
				}
			}
		case "navi_id":
			if meta.NaviID == identifierValue {
				return true, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate linked identifiers: %w", err)
	}
	return false, nil
}

// ── record_interaction ────────────────────────────────────────────────────────

func registerRecordInteraction(skillID string, db *sql.DB) {
	RegisterInternalHandler(skillID, "record_interaction", func(ctx context.Context, _ *SkillEntry, _ *Interface, args map[string]any) (any, error) {
		id := strings.TrimSpace(stringArg(args, "contact_id"))
		if id == "" {
			return nil, fmt.Errorf("contact_id is required")
		}
		kind := strings.TrimSpace(stringArg(args, "kind"))
		if kind == "" {
			return nil, fmt.Errorf("kind is required (e.g. message_sent)")
		}
		note := stringArg(args, "note")

		c, err := store.GetContact(ctx, db, id)
		if err != nil {
			return nil, err
		}
		var meta ContactMetadata
		if c.Metadata != "" && c.Metadata != "{}" {
			_ = json.Unmarshal([]byte(c.Metadata), &meta)
		}

		meta = appendInteraction(meta, kind, note)

		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		c.Metadata = string(metaJSON)
		if err := store.SaveContact(ctx, db, c); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":           true,
			"interaction_count": meta.InteractionCount,
			"last_interaction":  meta.LastInteraction,
		}, nil
	})
}

// appendInteraction appends an event to the interaction history (capped at 50).
func appendInteraction(meta ContactMetadata, kind, note string) ContactMetadata {
	const maxHistory = 50
	now := time.Now().UTC().Format(time.RFC3339)
	event := InteractionEvent{At: now, Kind: kind, Note: note}
	meta.InteractionHistory = append(meta.InteractionHistory, event)
	if len(meta.InteractionHistory) > maxHistory {
		meta.InteractionHistory = meta.InteractionHistory[len(meta.InteractionHistory)-maxHistory:]
	}
	meta.InteractionCount++
	meta.LastInteraction = now
	return meta
}

// ── helpers ───────────────────────────────────────────────────────────────────

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func intArg(args map[string]any, key string, def int) int {
	if args == nil {
		return def
	}
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

func boolArg(args map[string]any, key string, def bool) bool {
	if args == nil {
		return def
	}
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// buildMetadataFromArgs assembles a ContactMetadata from handler args.
func buildMetadataFromArgs(args map[string]any) ContactMetadata {
	return ContactMetadata{
		FirstName:    stringArg(args, "first_name"),
		LastName:     stringArg(args, "last_name"),
		Emails:       parseLabeledValues(args["emails"]),
		Phones:       parseLabeledValues(args["phones"]),
		Addresses:    parseAddresses(args["addresses"]),
		Organization: parseOrganization(args["organization"]),
		Birthday:     stringArg(args, "birthday"),
		Notes:        stringArg(args, "notes"),
		Tags:         parseStringSlice(args["tags"]),
		PhotoURL:     stringArg(args, "photo_url"),
	}
}

// parseLabeledValues converts []any (each item a map) into []LabeledValue.
func parseLabeledValues(raw any) []LabeledValue {
	if raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]LabeledValue, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		lv := LabeledValue{}
		if v, ok := m["value"].(string); ok {
			lv.Value = strings.TrimSpace(v)
		}
		if l, ok := m["label"].(string); ok {
			lv.Label = strings.TrimSpace(l)
		}
		if lv.Value != "" {
			out = append(out, lv)
		}
	}
	return out
}

// parseAddresses converts []any into []Address.
func parseAddresses(raw any) []Address {
	if raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]Address, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		a := Address{}
		for _, k := range []string{"street", "city", "state", "zip", "country", "label"} {
			if v, ok := m[k].(string); ok {
				switch k {
				case "street":
					a.Street = v
				case "city":
					a.City = v
				case "state":
					a.State = v
				case "zip":
					a.Zip = v
				case "country":
					a.Country = v
				case "label":
					a.Label = v
				}
			}
		}
		out = append(out, a)
	}
	return out
}

// parseOrganization converts a map arg into *Organization.
func parseOrganization(raw any) *Organization {
	if raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	org := &Organization{}
	if v, ok := m["company"].(string); ok {
		org.Company = strings.TrimSpace(v)
	}
	if v, ok := m["title"].(string); ok {
		org.Title = strings.TrimSpace(v)
	}
	if org.Company == "" && org.Title == "" {
		return nil
	}
	return org
}

// parseStringSlice converts []any to []string.
func parseStringSlice(raw any) []string {
	if raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// summarizeContacts returns a lightweight representation for list/search results.
func summarizeContacts(contacts []schema.Contact) []map[string]any {
	out := make([]map[string]any, 0, len(contacts))
	for _, c := range contacts {
		var meta ContactMetadata
		if c.Metadata != "" && c.Metadata != "{}" {
			_ = json.Unmarshal([]byte(c.Metadata), &meta)
		}
		item := map[string]any{
			"id":          c.ID,
			"name":        c.Name,
			"kind":        c.Kind,
			"owner_type":  c.OwnerType,
			"trust_level": c.TrustLevel,
			"updated_at":  c.UpdatedAt.Format(time.RFC3339),
		}
		// Surface the most useful fields for quick reference
		if len(meta.Emails) > 0 {
			item["primary_email"] = meta.Emails[0].Value
		}
		if len(meta.Phones) > 0 {
			item["primary_phone"] = meta.Phones[0].Value
		}
		if meta.Organization != nil && meta.Organization.Company != "" {
			item["company"] = meta.Organization.Company
		}
		out = append(out, item)
	}
	return out
}
