package projector

import (
	"sort"
	"strings"
	"time"
)

// IndexDir is the folder for read-only generated index pages (spec §6, §8).
// "Truth comes out as files. Derivations come out as indexes. Indexes are
// read-only." Edits to anything under this folder are ignored by the worker.
const IndexDir = "_index"

// IndexEntry is one row in a generated index page.
type IndexEntry struct {
	Title    string
	RelPath  string // path of the projected entity file, relative to the vault root
	When     time.Time
	Subtitle string
}

// RenderIndex renders a read-only index page. The frontmatter carries
// navi_readonly: true and navi_index: true so the page is unmistakably derived;
// the body lists entries as links. The page is never parsed back as an entity.
func RenderIndex(title string, entries []IndexEntry) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("navi_index: true\n")
	b.WriteString("navi_readonly: true\n")
	writeScalar(&b, "navi_title", title)
	b.WriteString(markerComment + "\n")
	b.WriteString("# This page is generated from the World Model and is read-only.\n")
	b.WriteString("# Edits here are ignored. Edit the entity files instead.\n")
	b.WriteString("---\n\n")
	b.WriteString("# " + strings.TrimSpace(title) + "\n\n")
	b.WriteString("> Read-only index, generated from canonical state. Edit the linked files, not this page.\n\n")
	if len(entries) == 0 {
		b.WriteString("_No entries yet._\n")
		return b.String()
	}
	for _, e := range entries {
		line := "- [" + mdEscape(e.Title) + "](" + linkPath(e.RelPath) + ")"
		if e.Subtitle != "" {
			line += " — " + mdEscape(e.Subtitle)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// ContactsByRecencyPath / KnowledgeByTopicPath are the two V1 index pages (spec
// §16: "ship one or two; let the rest grow with usage").
func ContactsByRecencyPath() string { return IndexDir + "/contacts-by-recency.md" }
func KnowledgeByTopicPath() string  { return IndexDir + "/knowledge-by-topic.md" }

// SortByRecency orders entries newest-first.
func SortByRecency(entries []IndexEntry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].When.After(entries[j].When) })
}

// IsIndexPath reports whether a vault-relative path is a generated index page.
func IsIndexPath(relPath string) bool {
	relPath = strings.TrimPrefix(filepathToSlash(relPath), "./")
	return strings.HasPrefix(relPath, IndexDir+"/") || relPath == IndexDir
}

func filepathToSlash(p string) string { return strings.ReplaceAll(p, "\\", "/") }

func linkPath(rel string) string {
	// Links in _index point up one level to the entity file.
	return "../" + strings.TrimPrefix(filepathToSlash(rel), "/")
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "]", "\\]")
	s = strings.ReplaceAll(s, "[", "\\[")
	return strings.TrimSpace(s)
}
