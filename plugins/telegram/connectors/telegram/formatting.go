package telegram

import (
	"regexp"
	"strconv"
	"strings"
)

// This file implements outbound markdown → Telegram-HTML rendering (T3-15).
//
// Telegram's "HTML" parse_mode renders a small tag set (<b>, <i>, <code>, <pre>,
// <a>, …) and requires &, <, > to be escaped in text. NAVI's chat replies are
// markdown, so we convert a safe subset to HTML. Anything we don't convert is
// escaped and shown literally. Callers fall back to plain text on a parse error
// (see telegramHTMLParseError), so a bad render can never block delivery.
//
// Deliberately converted: fenced code blocks, inline code, GitHub-style tables
// (rendered as aligned <pre>), bold (**/__), links, and ATX headings. Italic and
// strikethrough are intentionally NOT converted — single * / _ / ~ produce too
// many false positives (e.g. snake_case identifiers) and are low value.

var (
	fencedCodeRe = regexp.MustCompile("(?s)```([a-zA-Z0-9_+\\-]*)\\n?(.*?)```")
	inlineCodeRe = regexp.MustCompile("`([^`\\n]+)`")
	headingRe    = regexp.MustCompile(`(?m)^[ \t]{0,3}#{1,6}[ \t]+(.*?)[ \t]*#*[ \t]*$`)
	boldStarRe   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	boldUnderRe  = regexp.MustCompile(`__(.+?)__`)
	linkRe       = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^\s)]+)\)`)
)

func htmlEscapeTelegram(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func htmlEscapeTelegramAttr(s string) string {
	return strings.ReplaceAll(htmlEscapeTelegram(s), "\"", "&quot;")
}

// telegramHTMLParseError reports whether a Telegram API error came from failing to
// parse HTML entities, which is the signal to retry the send as plain text.
func telegramHTMLParseError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "can't parse entities") ||
		strings.Contains(lower, "can't find end") ||
		strings.Contains(lower, "unsupported start tag") ||
		strings.Contains(lower, "unclosed") ||
		strings.Contains(lower, "parse entities")
}

// markdownToTelegramHTML converts a markdown string to Telegram-flavored HTML.
func markdownToTelegramHTML(s string) string {
	var spans []string
	placeholder := func(i int) string { return "\x00C" + strconv.Itoa(i) + "\x00" }

	// 1. Fenced code blocks → <pre> (with optional language). Protected from all
	//    later transforms via placeholders.
	s = fencedCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := fencedCodeRe.FindStringSubmatch(m)
		lang, code := sub[1], strings.TrimSuffix(sub[2], "\n")
		var html string
		if lang != "" {
			html = "<pre><code class=\"language-" + htmlEscapeTelegramAttr(lang) + "\">" + htmlEscapeTelegram(code) + "</code></pre>"
		} else {
			html = "<pre>" + htmlEscapeTelegram(code) + "</pre>"
		}
		spans = append(spans, html)
		return placeholder(len(spans) - 1)
	})

	// 2. Inline code → <code>.
	s = inlineCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		code := strings.Trim(m, "`")
		spans = append(spans, "<code>"+htmlEscapeTelegram(code)+"</code>")
		return placeholder(len(spans) - 1)
	})

	// 3. Markdown tables → aligned <pre> block (Telegram can't render tables; a
	//    monospace <pre> at least preserves columns).
	s = flattenMarkdownTables(s, &spans, placeholder)

	// 4. Escape everything that remains.
	s = htmlEscapeTelegram(s)

	// 5. Inline transforms on escaped text (order: headings, links, bold).
	s = headingRe.ReplaceAllString(s, "<b>$1</b>")
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)
	s = boldStarRe.ReplaceAllString(s, "<b>$1</b>")
	s = boldUnderRe.ReplaceAllString(s, "<b>$1</b>")

	// 6. Reinsert protected spans.
	for i, span := range spans {
		s = strings.ReplaceAll(s, placeholder(i), span)
	}
	return s
}

func parseTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isTableSeparatorLine(line string) bool {
	if !strings.Contains(line, "-") {
		return false
	}
	cells := parseTableCells(line)
	if len(cells) < 1 {
		return false
	}
	for _, c := range cells {
		if c == "" || !strings.Contains(c, "-") {
			return false
		}
		for _, r := range c {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

// flattenMarkdownTables detects GitHub-style tables and replaces each with a
// placeholder pointing at an aligned <pre> rendering. Operates before escaping;
// the produced <pre> content is pre-escaped so the later escape pass skips it.
func flattenMarkdownTables(s string, spans *[]string, placeholder func(int) string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) && strings.Contains(lines[i], "|") && isTableSeparatorLine(lines[i+1]) {
			header := parseTableCells(lines[i])
			rows := [][]string{header}
			j := i + 2
			for j < len(lines) && strings.Contains(lines[j], "|") {
				rows = append(rows, parseTableCells(lines[j]))
				j++
			}
			*spans = append(*spans, "<pre>"+htmlEscapeTelegram(renderAlignedTable(rows))+"</pre>")
			out = append(out, placeholder(len(*spans)-1))
			i = j - 1
			continue
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
}

func renderAlignedTable(rows [][]string) string {
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for c := 0; c < cols; c++ {
			if c < len(r) && len([]rune(r[c])) > widths[c] {
				widths[c] = len([]rune(r[c]))
			}
		}
	}
	var b strings.Builder
	for ri, r := range rows {
		cells := make([]string, cols)
		for c := 0; c < cols; c++ {
			val := ""
			if c < len(r) {
				val = r[c]
			}
			pad := widths[c] - len([]rune(val))
			if pad < 0 {
				pad = 0
			}
			cells[c] = val + strings.Repeat(" ", pad)
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, " | "), " "))
		b.WriteString("\n")
		if ri == 0 {
			// underline the header row
			sep := make([]string, cols)
			for c := 0; c < cols; c++ {
				sep[c] = strings.Repeat("-", widths[c])
			}
			b.WriteString(strings.Join(sep, "-+-"))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
