package canonicalize

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// trackingParams are query parameters stripped during URL normalization because
// they carry no addressable meaning (analytics / click attribution).
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"gclid":        true,
	"fbclid":       true,
	"mc_eid":       true,
	"mc_cid":       true,
	"igshid":       true,
	"ref":          true,
	"ref_src":      true,
}

var defaultPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

// NormalizeURL canonicalizes a single URL: lowercases scheme and host, removes
// the fragment, strips tracking query parameters, drops default ports, and sorts
// the remaining query parameters for stability. Inputs that do not parse as
// absolute URLs are returned trimmed but otherwise unchanged.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Fragment = ""

	host := strings.ToLower(u.Host)
	if h, port, ok := splitHostPort(host); ok {
		if defaultPorts[u.Scheme] == port {
			host = h
		}
	}
	u.Host = host

	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if trackingParams[strings.ToLower(k)] {
				q.Del(k)
			}
		}
		// url.Values.Encode already sorts keys; assigning keeps determinism.
		u.RawQuery = encodeSorted(q)
	}

	// Drop a trailing slash on bare-root paths for stability ("/" -> "").
	if u.Path == "/" {
		u.Path = ""
	}
	return u.String()
}

func splitHostPort(host string) (string, string, bool) {
	i := strings.LastIndexByte(host, ':')
	if i < 0 {
		return host, "", false
	}
	// Avoid mis-splitting IPv6 literals like "[::1]".
	if strings.Contains(host[:i], "]") || !strings.Contains(host, "]") && strings.Count(host, ":") == 1 {
		return host[:i], host[i+1:], true
	}
	return host, "", false
}

func encodeSorted(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	first := true
	for _, k := range keys {
		vals := v[k]
		sort.Strings(vals)
		for _, val := range vals {
			if !first {
				b.WriteByte('&')
			}
			first = false
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(val))
		}
	}
	return b.String()
}

// urlInTextRE matches http(s) URLs embedded in free text.
var urlInTextRE = regexp.MustCompile(`https?://[^\s<>()\[\]"']+`)

// normalizeURLsInText rewrites every http(s) URL found in s to its normalized
// form. Trailing punctuation that is clearly sentence punctuation (".,;:!?") is
// preserved outside the normalized URL.
func normalizeURLsInText(s string) string {
	return urlInTextRE.ReplaceAllStringFunc(s, func(m string) string {
		trailing := ""
		for len(m) > 0 {
			last := m[len(m)-1]
			if strings.IndexByte(".,;:!?)", last) >= 0 {
				trailing = string(last) + trailing
				m = m[:len(m)-1]
				continue
			}
			break
		}
		return NormalizeURL(m) + trailing
	})
}
