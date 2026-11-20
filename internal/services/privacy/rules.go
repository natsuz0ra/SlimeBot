package privacy

import (
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type span struct {
	start, end   int
	kind, source string
}
type detectorRule struct {
	kind  string
	re    *regexp.Regexp
	group int
	check func(string) bool
}

// These patterns describe public credential formats and local data conventions.
// A candidate is checked again before it changes request content.
var builtins = []detectorRule{
	{"PRIVATE_KEY", regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY(?: BLOCK)?-----.*?-----END [A-Z ]*PRIVATE KEY(?: BLOCK)?-----`), 0, nil},
	{"API_KEY", regexp.MustCompile(`\b(?:sk-(?:[a-z]+-)?[A-Za-z0-9_-]{16,}|(?:gh[pousr]_|github_pat_|glpat-|hf_|gsk_|xai-|npm_|pypi-|ark-)[A-Za-z0-9_-]{16,}|[a-z]{2,4}_sk_[A-Za-z0-9_-]{16,}|AIza[A-Za-z0-9_-]{35}|(?:AKIA|ASIA)[A-Z0-9]{16}|xox[baprs]-[A-Za-z0-9-]{12,}|[sr]k_live_[A-Za-z0-9]{16,})`), 0, nil},
	{"TOKEN", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]{8,}`), 0, nil},
	{"PASSWORD", regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s:/@]*:([^\s/@]{3,})@`), 1, nil},
	{"TOKEN", regexp.MustCompile(`(?i)\bBearer[ \t]+([A-Za-z0-9._~+/=-]{8,})`), 1, nil},
	{"SECRET", regexp.MustCompile(`(?im)^[ \t]*(?:authorization|cookie)[ \t]*:[ \t]*([^\r\n]+)`), 1, nil},
	{"EMAIL", regexp.MustCompile(`[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?\.[A-Za-z]{2,}`), 0, realEmail},
	{"PHONE", regexp.MustCompile(`(?:\+?86[ -]?)?1[3-9][0-9]{9}`), 0, nil},
	{"ID_CARD", regexp.MustCompile(`[1-9][0-9]{5}(?:19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]`), 0, chineseID},
	{"BANK_CARD", regexp.MustCompile(`[3-6][0-9]{3}(?:[ -]?[0-9]{4}){2,3}(?:[ -]?[0-9]{1,3})?`), 0, luhn},
	{"SSN", regexp.MustCompile(`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`), 0, func(v string) bool {
		return !strings.HasPrefix(v, "000") && !strings.HasPrefix(v, "666") && v[4:6] != "00" && v[7:] != "0000"
	}},
	{"MAC", regexp.MustCompile(`(?i)\b(?:[a-f0-9]{2}[:-]){5}[a-f0-9]{2}\b`), 0, nil},
	{"IP", regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`), 0, func(v string) bool {
		ip, e := netip.ParseAddr(v)
		return e == nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast()
	}},
	{"IBAN", regexp.MustCompile(`\b[A-Z]{2}[0-9]{2}(?:[ ]?[A-Z0-9]){11,30}\b`), 0, iban},
	{"PASSPORT", regexp.MustCompile(`(?i)(?:passport|护照)[ \t]*(?:no\.?|号码|号)?[ \t]*[:：=#]?[ \t]*([A-Z0-9]{6,12})`), 1, nil},
	{"BIRTHDAY", regexp.MustCompile(`(?i)(?:birthday|date of birth|dob|生日|出生日期)[ \t]*[:：=]?[ \t]*((?:19|20)[0-9]{2}[-/.][01]?[0-9][-/\.][0-3]?[0-9])`), 1, nil},
	{"SERIAL", regexp.MustCompile(`(?i)(?:serial(?: number)?|序列号)[ \t]*[:：=][ \t]*([A-Z0-9-]{6,40})`), 1, nil},
	{"USERNAME", regexp.MustCompile(`(?:/Users/|/home/|[A-Za-z]:\\Users\\)([^/\\\s"<>]+)`), 1, nil},
	{"HOSTNAME", regexp.MustCompile(`\b[A-Za-z0-9._-]+@([A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*)`), 1, nil},
	{"BUCKET", regexp.MustCompile(`(?:s3|gs|oss)://([a-z0-9][a-z0-9.-]{2,62})`), 1, nil},
}

var assignments = regexp.MustCompile(`(?im)([A-Za-z_][A-Za-z0-9_.-]*|[\p{Han}]*密码|[\p{Han}]*密碼|口令|密钥|令牌)["' \t]*(?:[:=：]|是|为|為)[ \t]*["']?([^\s"'` + "`" + `,;，。；<>\[\]{}]+)`)

func secretField(name string) bool {
	n := strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	if strings.Contains(n, "password") || strings.Contains(n, "passwd") || strings.Contains(n, "secret") || strings.Contains(n, "token") || strings.Contains(n, "api_key") || strings.Contains(n, "apikey") || strings.Contains(n, "密码") || strings.Contains(n, "密碼") {
		return true
	}
	switch n {
	case "key", "pwd", "pass", "authorization", "cookie", "口令", "密钥", "令牌":
		return true
	}
	return (name == strings.ToUpper(name)) && (strings.HasSuffix(name, "_KEY") || strings.HasSuffix(name, "_PASS"))
}

func realSecret(v string) bool {
	if len(v) < 4 || strings.HasPrefix(v, "$") || strings.HasPrefix(v, "{{") || strings.Trim(v, "*.") == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "null", "true", "false", "none", "password", "changeme", "your_api_key", "example":
		return false
	}
	return true
}

func realEmail(v string) bool {
	_, domain, ok := strings.Cut(strings.ToLower(v), "@")
	if !ok {
		return false
	}
	for _, d := range []string{"example.com", "example.org", "example.net", "test", "invalid", "localhost"} {
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return false
		}
	}
	return true
}

func luhn(v string) bool {
	v = strings.NewReplacer(" ", "", "-", "").Replace(v)
	if len(v) < 13 || len(v) > 19 {
		return false
	}
	sum := 0
	for i := len(v) - 1; i >= 0; i-- {
		n := int(v[i] - '0')
		if n < 0 || n > 9 {
			return false
		}
		if (len(v)-1-i)%2 == 1 {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
	}
	return sum%10 == 0 && strings.Trim(v, "0") != ""
}

func chineseID(v string) bool {
	if len(v) != 18 {
		return false
	}
	if _, e := time.Parse("20060102", v[6:14]); e != nil {
		return false
	}
	weights := [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	sum := 0
	for i, w := range weights {
		sum += int(v[i]-'0') * w
	}
	return strings.ToUpper(v[17:]) == string("10X98765432"[sum%11])
}

func iban(v string) bool {
	v = strings.ReplaceAll(v, " ", "")
	if len(v) < 15 || len(v) > 34 {
		return false
	}
	v = v[4:] + v[:4]
	n := 0
	for _, c := range v {
		s := string(c)
		if c >= 'A' && c <= 'Z' {
			s = strconv.Itoa(int(c-'A') + 10)
		}
		for _, d := range s {
			if d < '0' || d > '9' {
				return false
			}
			n = (n*10 + int(d-'0')) % 97
		}
	}
	return n == 1
}

func wordAt(s string, i int, before bool) bool {
	if i <= 0 && before || i >= len(s) && !before {
		return false
	}
	var r rune
	if before {
		r, _ = utf8.DecodeLastRuneInString(s[:i])
	} else {
		r, _ = utf8.DecodeRuneInString(s[i:])
	}
	return (unicode.IsLetter(r) && !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)) || unicode.IsDigit(r) || r == '_'
}

func candidates(text string, c Config) []span {
	var out []span
	for _, r := range builtins {
		if !c.covers(r.kind) {
			continue
		}
		for _, m := range r.re.FindAllStringSubmatchIndex(text, 20000) {
			a, b := m[r.group*2], m[r.group*2+1]
			if a < 0 || a == b {
				continue
			}
			value := text[a:b]
			if r.check != nil && !r.check(value) {
				continue
			}
			if (r.kind == "PHONE" || r.kind == "ID_CARD" || r.kind == "BANK_CARD" || r.kind == "API_KEY" || r.kind == "TOKEN") && (wordAt(text, a, true) || wordAt(text, b, false)) {
				continue
			}
			out = append(out, span{a, b, r.kind, "rule"})
		}
	}
	if c.Secrets {
		for _, m := range assignments.FindAllStringSubmatchIndex(text, 20000) {
			name, value := text[m[2]:m[3]], text[m[4]:m[5]]
			if secretField(name) && realSecret(value) {
				kind := "SECRET"
				if strings.Contains(strings.ToLower(name), "pass") || strings.Contains(name, "密码") || strings.Contains(name, "密碼") {
					kind = "PASSWORD"
				}
				if c.covers(kind) {
					out = append(out, span{m[4], m[5], kind, "rule"})
				}
			}
		}
	}
	for _, w := range c.Words {
		if !c.covers("TERM") {
			break
		}
		pattern := regexp.QuoteMeta(w)
		if c.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		for _, m := range regexp.MustCompile(pattern).FindAllStringIndex(text, 20000) {
			if !c.WholeWords || !wordAt(text, m[0], true) && !wordAt(text, m[1], false) {
				out = append(out, span{m[0], m[1], "TERM", "word"})
			}
		}
	}
	if c.Secrets {
		for _, r := range c.Rules {
			if !c.covers(r.Kind) {
				continue
			}
			pattern := r.Pattern
			if pattern == "" {
				pattern = regexp.QuoteMeta(r.Prefix) + `[A-Za-z0-9_+/=-]{4,}`
			}
			re := regexp.MustCompile(pattern)
			for _, m := range re.FindAllStringSubmatchIndex(text, 20000) {
				a, b := m[0], m[1]
				if re.NumSubexp() > 0 {
					a, b = m[2], m[3]
				}
				if a >= 0 && a < b {
					out = append(out, span{a, b, r.Kind, "custom"})
				}
			}
		}
	}
	return out
}

func ordered(spans []span) []span {
	priority := func(kind string) int {
		switch kind {
		case "PRIVATE_KEY":
			return 4
		case "API_KEY", "TOKEN":
			return 3
		case "PASSWORD", "SECRET":
			return 2
		default:
			return 1
		}
	}
	sort.SliceStable(spans, func(i, j int) bool {
		a, b := spans[i], spans[j]
		if priority(a.kind) != priority(b.kind) {
			return priority(a.kind) > priority(b.kind)
		}
		if a.start == b.start {
			return a.end > b.end
		}
		return a.start < b.start
	})
	selected := make([]span, 0, len(spans))
	for _, candidate := range spans {
		index := sort.Search(len(selected), func(i int) bool { return selected[i].start >= candidate.start })
		if index > 0 && selected[index-1].end > candidate.start {
			continue
		}
		if index < len(selected) && selected[index].start < candidate.end {
			continue
		}
		selected = append(selected, span{})
		copy(selected[index+1:], selected[index:])
		selected[index] = candidate
	}
	return selected
}
