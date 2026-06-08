package handlers

import (
	"fmt"
	"mime/quotedprintable"
	"strings"
	"unicode/utf8"
)

// ContactMetadata is the structured payload stored in contacts.metadata (JSON).
type ContactMetadata struct {
	FirstName    string         `json:"first_name,omitempty"`
	LastName     string         `json:"last_name,omitempty"`
	Emails       []LabeledValue `json:"emails,omitempty"`
	Phones       []LabeledValue `json:"phones,omitempty"`
	Addresses    []Address      `json:"addresses,omitempty"`
	Organization *Organization  `json:"organization,omitempty"`
	Birthday     string         `json:"birthday,omitempty"` // YYYY-MM-DD
	Notes        string         `json:"notes,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	Source       string         `json:"source,omitempty"`
	PhotoURL     string         `json:"photo_url,omitempty"`

	// Identity & interaction fields (folded in from navi.contacts.manager)
	NaviID             string             `json:"navi_id,omitempty"`          // NAVI-to-NAVI identifier
	LastInteraction    string             `json:"last_interaction,omitempty"` // ISO 8601 timestamp
	InteractionCount   int                `json:"interaction_count,omitempty"`
	InteractionHistory []InteractionEvent `json:"interaction_history,omitempty"` // capped at 50
}

// InteractionEvent records a single timestamped interaction with a contact.
type InteractionEvent struct {
	At   string `json:"at"`             // ISO 8601 UTC
	Kind string `json:"kind"`           // e.g. "message_sent", "contact_created"
	Note string `json:"note,omitempty"` // free-form detail
}

// LabeledValue pairs a string value with an optional label (e.g. "work", "home").
type LabeledValue struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

// Address holds a postal address with an optional label.
type Address struct {
	Street  string `json:"street,omitempty"`
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	Zip     string `json:"zip,omitempty"`
	Country string `json:"country,omitempty"`
	Label   string `json:"label,omitempty"`
}

// Organization holds company and title fields.
type Organization struct {
	Company string `json:"company,omitempty"`
	Title   string `json:"title,omitempty"`
}

// ParsedVCard is the result of parsing a single vCard block.
type ParsedVCard struct {
	// Name is the FN: value — used as schema.Contact.Name.
	Name     string
	Kind     string // always "person" from vCard; callers may override
	Metadata ContactMetadata
}

// ContactToVCF converts a contact name + metadata to a vCard 3.0 string.
// The returned string includes BEGIN:VCARD and END:VCARD delimiters with CRLF line endings.
func ContactToVCF(name string, meta ContactMetadata) string {
	var b strings.Builder

	line := func(s string) {
		b.WriteString(s)
		b.WriteString("\r\n")
	}

	line("BEGIN:VCARD")
	line("VERSION:3.0")

	// FN (formatted name) — required
	line("FN:" + vcfEscape(name))

	// N (structured name) — Last;First;Middle;Prefix;Suffix
	last := meta.LastName
	first := meta.FirstName
	if last == "" && first == "" {
		first, last = splitDisplayName(name)
	}
	line(fmt.Sprintf("N:%s;%s;;;", vcfEscape(last), vcfEscape(first)))

	// EMAIL
	for _, e := range meta.Emails {
		if e.Value == "" {
			continue
		}
		typeStr := emailTypeParam(e.Label)
		line(fmt.Sprintf("EMAIL;TYPE=%s:%s", typeStr, vcfEscape(e.Value)))
	}

	// TEL
	for _, p := range meta.Phones {
		if p.Value == "" {
			continue
		}
		typeStr := phoneTypeParam(p.Label)
		line(fmt.Sprintf("TEL;TYPE=%s:%s", typeStr, vcfEscape(p.Value)))
	}

	// ADR — 7-part: PO box;extended;street;city;state;zip;country
	for _, a := range meta.Addresses {
		if a.Street == "" && a.City == "" && a.Country == "" {
			continue
		}
		typeStr := addrTypeParam(a.Label)
		adr := fmt.Sprintf(";;%s;%s;%s;%s;%s",
			vcfEscapeADR(a.Street),
			vcfEscapeADR(a.City),
			vcfEscapeADR(a.State),
			vcfEscapeADR(a.Zip),
			vcfEscapeADR(a.Country),
		)
		line(fmt.Sprintf("ADR;TYPE=%s:%s", typeStr, adr))
	}

	// ORG
	if meta.Organization != nil && meta.Organization.Company != "" {
		line("ORG:" + vcfEscape(meta.Organization.Company))
		if meta.Organization.Title != "" {
			line("TITLE:" + vcfEscape(meta.Organization.Title))
		}
	}

	// BDAY — strip dashes: 1990-01-15 → 19900115
	if meta.Birthday != "" {
		bday := strings.ReplaceAll(meta.Birthday, "-", "")
		line("BDAY:" + bday)
	}

	// NOTE — fold long lines at 75 chars
	if meta.Notes != "" {
		writeVCFNote(&b, meta.Notes)
	}

	line("END:VCARD")
	return b.String()
}

// ParseVCF parses one or more vCard records from raw VCF text.
// Returns a slice of parsed cards and a slice of non-fatal error strings for skipped cards.
func ParseVCF(vcf string) ([]ParsedVCard, []string) {
	// Normalize line endings
	normalized := strings.ReplaceAll(vcf, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	// Split into individual vCard blocks
	blocks := splitVCards(normalized)

	var cards []ParsedVCard
	var errs []string

	for i, block := range blocks {
		card, err := parseOneVCard(block)
		if err != nil {
			errs = append(errs, fmt.Sprintf("card %d: %v", i+1, err))
			continue
		}
		cards = append(cards, card)
	}
	return cards, errs
}

// splitVCards splits normalized (LF-only) VCF text into individual vCard blocks.
func splitVCards(text string) []string {
	var blocks []string
	var current strings.Builder
	inCard := false

	for _, line := range strings.Split(text, "\n") {
		upper := strings.ToUpper(strings.TrimSpace(line))
		if upper == "BEGIN:VCARD" {
			inCard = true
			current.Reset()
			current.WriteString(line)
			current.WriteByte('\n')
		} else if upper == "END:VCARD" {
			if inCard {
				current.WriteString(line)
				current.WriteByte('\n')
				blocks = append(blocks, current.String())
				current.Reset()
				inCard = false
			}
		} else if inCard {
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}
	return blocks
}

// parseOneVCard parses a single vCard block (LF line endings, no BEGIN/END needed but tolerated).
func parseOneVCard(block string) (ParsedVCard, error) {
	lines := unfoldLines(strings.Split(block, "\n"))

	var card ParsedVCard
	card.Kind = "person"

	for _, raw := range lines {
		raw = strings.TrimRight(raw, "\r")
		if raw == "" {
			continue
		}
		upperRaw := strings.ToUpper(raw)
		if upperRaw == "BEGIN:VCARD" || upperRaw == "END:VCARD" {
			continue
		}

		// Split property name+params from value on first colon
		colonIdx := strings.Index(raw, ":")
		if colonIdx < 0 {
			continue
		}
		propPart := raw[:colonIdx]
		value := raw[colonIdx+1:]

		// Decode quoted-printable if needed
		if strings.Contains(strings.ToUpper(propPart), "ENCODING=QUOTED-PRINTABLE") {
			decoded, err := decodeQuotedPrintable(value)
			if err == nil {
				value = decoded
			}
		}

		// Parse property name and TYPE params
		parts := strings.Split(propPart, ";")
		propName := strings.ToUpper(strings.TrimSpace(parts[0]))
		params := parts[1:]

		switch propName {
		case "FN":
			card.Name = vcfUnescape(value)

		case "N":
			// N:Last;First;Middle;Prefix;Suffix
			nParts := strings.SplitN(value, ";", 5)
			if len(nParts) >= 1 {
				card.Metadata.LastName = vcfUnescape(nParts[0])
			}
			if len(nParts) >= 2 {
				card.Metadata.FirstName = vcfUnescape(nParts[1])
			}

		case "EMAIL":
			label := extractTypeParam(params, "work")
			card.Metadata.Emails = append(card.Metadata.Emails, LabeledValue{
				Value: vcfUnescape(value),
				Label: normalizeEmailLabel(label),
			})

		case "TEL":
			label := extractTypeParam(params, "voice")
			card.Metadata.Phones = append(card.Metadata.Phones, LabeledValue{
				Value: vcfUnescape(value),
				Label: normalizePhoneLabel(label),
			})

		case "ADR":
			label := extractTypeParam(params, "home")
			// ADR: PO box;extended;street;city;state;zip;country
			adrParts := strings.SplitN(value, ";", 7)
			addr := Address{Label: normalizeAddrLabel(label)}
			if len(adrParts) >= 3 {
				addr.Street = vcfUnescape(adrParts[2])
			}
			if len(adrParts) >= 4 {
				addr.City = vcfUnescape(adrParts[3])
			}
			if len(adrParts) >= 5 {
				addr.State = vcfUnescape(adrParts[4])
			}
			if len(adrParts) >= 6 {
				addr.Zip = vcfUnescape(adrParts[5])
			}
			if len(adrParts) >= 7 {
				addr.Country = vcfUnescape(adrParts[6])
			}
			card.Metadata.Addresses = append(card.Metadata.Addresses, addr)

		case "ORG":
			// Take only the first component (ORG:Company;Department)
			orgParts := strings.SplitN(value, ";", 2)
			if card.Metadata.Organization == nil {
				card.Metadata.Organization = &Organization{}
			}
			card.Metadata.Organization.Company = vcfUnescape(orgParts[0])

		case "TITLE":
			if card.Metadata.Organization == nil {
				card.Metadata.Organization = &Organization{}
			}
			card.Metadata.Organization.Title = vcfUnescape(value)

		case "BDAY":
			// Normalize 19900115 → 1990-01-15
			card.Metadata.Birthday = normalizeBirthday(value)

		case "NOTE":
			card.Metadata.Notes = vcfUnescape(value)

		case "PHOTO":
			// Skip binary photo data; leave PhotoURL empty
		}
	}

	if strings.TrimSpace(card.Name) == "" {
		return ParsedVCard{}, fmt.Errorf("missing FN property")
	}
	card.Metadata.Source = "vcard"
	return card, nil
}

// unfoldLines joins continuation lines (RFC 6350: line starting with space/tab continues previous).
func unfoldLines(lines []string) []string {
	var out []string
	for _, line := range lines {
		if len(line) == 0 {
			out = append(out, line)
			continue
		}
		r, _ := utf8.DecodeRuneInString(line)
		if (r == ' ' || r == '\t') && len(out) > 0 {
			out[len(out)-1] += line[1:] // strip leading whitespace
		} else {
			out = append(out, line)
		}
	}
	return out
}

// writeVCFNote writes a NOTE property with RFC 6350 line folding at 75 chars.
func writeVCFNote(b *strings.Builder, note string) {
	escaped := vcfEscape(note)
	prefix := "NOTE:"
	line := prefix + escaped

	if len(line) <= 75 {
		b.WriteString(line)
		b.WriteString("\r\n")
		return
	}

	// Write first segment
	b.WriteString(line[:75])
	b.WriteString("\r\n")
	remaining := line[75:]
	for len(remaining) > 0 {
		chunk := remaining
		if len(chunk) > 74 { // 74 + 1 leading space = 75
			chunk = remaining[:74]
		}
		b.WriteByte(' ')
		b.WriteString(chunk)
		b.WriteString("\r\n")
		remaining = remaining[len(chunk):]
	}
}

// splitDisplayName splits "First Last" into (first, last).
// Uses the last token as last name to handle "John van Doe" → first="John van", last="Doe".
func splitDisplayName(name string) (first, last string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	idx := strings.LastIndex(name, " ")
	if idx < 0 {
		return "", name
	}
	return name[:idx], name[idx+1:]
}

// vcfEscape escapes special characters in a vCard TEXT value.
func vcfEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// vcfEscapeADR escapes for ADR field parts (semicolons separate parts, not escaped within a part).
func vcfEscapeADR(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// vcfUnescape reverses vCard TEXT escaping.
func vcfUnescape(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\N", "\n")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// emailTypeParam returns the vCard TYPE string for an email label.
func emailTypeParam(label string) string {
	switch strings.ToLower(label) {
	case "home":
		return "INTERNET,HOME"
	case "other", "":
		return "INTERNET,OTHER"
	default: // work, work email, etc.
		return "INTERNET,WORK"
	}
}

// phoneTypeParam returns the vCard TYPE string for a phone label.
func phoneTypeParam(label string) string {
	switch strings.ToLower(label) {
	case "home":
		return "HOME,VOICE"
	case "work", "office":
		return "WORK,VOICE"
	case "fax":
		return "FAX"
	case "pager":
		return "PAGER"
	case "mobile", "cell", "":
		return "CELL"
	default:
		return "VOICE"
	}
}

// addrTypeParam returns the vCard TYPE string for an address label.
func addrTypeParam(label string) string {
	switch strings.ToLower(label) {
	case "work", "office":
		return "WORK"
	default:
		return "HOME"
	}
}

// extractTypeParam finds the first TYPE= value from a slice of vCard property params.
func extractTypeParam(params []string, defaultVal string) string {
	for _, p := range params {
		upper := strings.ToUpper(p)
		if strings.HasPrefix(upper, "TYPE=") {
			return strings.TrimPrefix(upper, "TYPE=")
		}
		// Some exporters use bare type values without "TYPE=" prefix
		switch upper {
		case "WORK", "HOME", "CELL", "MOBILE", "VOICE", "FAX", "PAGER", "OTHER", "INTERNET":
			return upper
		}
	}
	return strings.ToUpper(defaultVal)
}

// normalizeEmailLabel converts vCard type strings to our simple labels.
func normalizeEmailLabel(typeStr string) string {
	t := strings.ToLower(typeStr)
	if strings.Contains(t, "home") {
		return "home"
	}
	if strings.Contains(t, "work") || strings.Contains(t, "internet") {
		return "work"
	}
	return "other"
}

// normalizePhoneLabel converts vCard type strings to our simple labels.
func normalizePhoneLabel(typeStr string) string {
	t := strings.ToLower(typeStr)
	if strings.Contains(t, "cell") || strings.Contains(t, "mobile") {
		return "mobile"
	}
	if strings.Contains(t, "home") {
		return "home"
	}
	if strings.Contains(t, "work") {
		return "work"
	}
	if strings.Contains(t, "fax") {
		return "fax"
	}
	return "voice"
}

// normalizeAddrLabel converts vCard type strings to our simple labels.
func normalizeAddrLabel(typeStr string) string {
	t := strings.ToLower(typeStr)
	if strings.Contains(t, "work") {
		return "work"
	}
	return "home"
}

// normalizeBirthday converts 19900115 to 1990-01-15; passes YYYY-MM-DD through unchanged.
func normalizeBirthday(s string) string {
	s = strings.TrimSpace(s)
	// Already in YYYY-MM-DD form
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return s
	}
	// YYYYMMDD form
	if len(s) == 8 {
		return s[0:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	return s
}

// decodeQuotedPrintable decodes a quoted-printable encoded string.
func decodeQuotedPrintable(s string) (string, error) {
	r := quotedprintable.NewReader(strings.NewReader(s))
	var b strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			b.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return b.String(), nil
}
