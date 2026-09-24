package convert

import "strings"

// Mode is the serialization mode HTTPie uses for data fields.
type Mode int

const (
	ModeJSON Mode = iota
	ModeForm
	ModeMultipart
)

// Kind classifies one HTTPie request item.
type Kind int

const (
	KindHeader Kind = iota
	KindData
	KindRawJSON
	KindQuery
	KindFile
)

// Item is a parsed HTTPie request item: a header, data field, query parameter
// or file upload.
type Item struct {
	Kind     Kind
	Name     string
	Value    string
	FromFile bool // value came from a file (the "@" suffix forms)
	Unset    bool // "Header:" — drop a header HTTPie would otherwise send
	Empty    bool // "Header;" — send the header with an empty value
}

type separator struct {
	text     string
	kind     Kind
	fromFile bool
}

// Longest separators come first so ":=" wins over "=" and ":=@" over ":=".
var separators = []separator{
	{":=@", KindRawJSON, true},
	{"==@", KindQuery, true},
	{":@", KindHeader, true},
	{"=@", KindData, true},
	{":=", KindRawJSON, false},
	{"==", KindQuery, false},
	{":", KindHeader, false},
	{"=", KindData, false},
	{"@", KindFile, false},
}

// splitItem splits tok on its first unescaped separator. ok is false when tok
// contains no separator at all.
func splitItem(tok string) (it Item, ok bool) {
	best := -1
	var bestSep separator
	for _, s := range separators {
		idx := indexUnescaped(tok, s.text)
		if idx < 0 {
			continue
		}
		if best == -1 || idx < best {
			best, bestSep = idx, s
		}
	}
	if best == -1 {
		return Item{}, false
	}
	it = Item{
		Kind:     bestSep.kind,
		Name:     unescape(tok[:best]),
		Value:    unescape(tok[best+len(bestSep.text):]),
		FromFile: bestSep.fromFile,
	}
	if it.Kind == KindHeader && it.Value == "" && !it.FromFile {
		it.Unset = true
	}
	return it, true
}

// indexUnescaped returns the index of the first occurrence of sep that is not
// preceded by an odd number of backslashes, or -1.
func indexUnescaped(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] != sep {
			continue
		}
		if isEscaped(s, i) {
			continue
		}
		return i
	}
	return -1
}

func isEscaped(s string, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// unescape removes the backslash escapes HTTPie uses to protect separator
// characters (e.g. "foo\==bar" is the data field "foo=").
func unescape(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var schemes = map[string]bool{
	"http": true, "https": true, "ftp": true, "ftps": true,
	"ws": true, "wss": true, "file": true, "gopher": true, "tftp": true,
}

func isScheme(s string) bool { return schemes[s] }

// isMethodToken reports whether tok looks like an HTTP method (all upper case
// ASCII letters). HTTPie allows custom methods, so the check is deliberately
// loose; it only runs before the URL has been seen.
func isMethodToken(tok string) bool {
	if tok == "" {
		return false
	}
	for i := 0; i < len(tok); i++ {
		if tok[i] < 'A' || tok[i] > 'Z' {
			return false
		}
	}
	return true
}

func isHeaderName(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t;:@=")
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// looksLikeURL disambiguates the URL position from request items. The hard
// cases are "host:port/path" (a URL) against "Name:Value" (a header).
func looksLikeURL(tok string, isItem bool) bool {
	if i := strings.Index(tok, "://"); i > 0 && isScheme(tok[:i]) {
		return true
	}
	if !isItem {
		// No separator at all: this is the URL ("pie.dev/post").
		return true
	}
	i := indexUnescaped(tok, ":")
	if i < 0 {
		return false
	}
	host, rest := tok[:i], tok[i+1:]
	port := rest
	if j := strings.IndexAny(port, "/?#"); j >= 0 {
		port = port[:j]
	}
	if !allDigits(port) {
		return false
	}
	return host == "localhost" || strings.Contains(host, ".")
}
