package convert

import (
	"fmt"
	"strings"
)

// Options holds this tool's own knobs. They are deliberately not HTTPie
// options, and main only recognises them as "--h2c-*" arguments.
type Options struct {
	Command         string // HTTPie program name: "http" (default) or "https"; selects the default scheme
	CurlBinary      string // argv[0] of the emitted command; empty means "curl"
	WithDefaults    bool   // also emit HTTPie's default Accept / Accept-Encoding headers
	SkipUnsupported bool   // warn instead of failing on HTTPie options curl cannot express
	Multiline       bool   // print over multiple lines with backslashes
}

// Request is a parsed HTTPie command line.
type Request struct {
	Method         string
	MethodExplicit bool
	URL            string
	Scheme         string
	Items          []Item
	Mode           Mode
	RawBody        string
	HasRawBody     bool

	Verbose      bool
	Follow       bool
	MaxRedirects string
	Insecure     bool
	Proxy        string
	Timeout      string
	PathAsIs     bool
	Stream       bool
	Output       string
	Auth         string
	Bearer       string

	// Unsupported lists HTTPie options with no curl equivalent. main decides
	// whether that is fatal (default) or just a warning.
	Unsupported []string

	acceptJSON bool
}

func defaultScheme(command string) string {
	if command == "https" {
		return "https"
	}
	return "http"
}

// Parse turns an HTTPie argument list (without the program name) into a Request.
func Parse(args []string, opts Options) (*Request, error) {
	r := &Request{Scheme: defaultScheme(opts.Command)}
	var sawURL, sawMethod, literal bool
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" && !literal {
			literal = true
			continue
		}
		if !literal && strings.HasPrefix(tok, "-") && tok != "-" {
			if err := r.option(args, &i, tok); err != nil {
				return nil, err
			}
			continue
		}
		// "http ://example.org" — keep the "://" the shell-user pasted.
		if !sawURL && strings.HasPrefix(tok, "://") {
			r.URL, sawURL = tok[3:], true
			continue
		}
		if !sawMethod && !sawURL && isMethodToken(tok) {
			r.Method, r.MethodExplicit, sawMethod = tok, true, true
			continue
		}
		it, isItem := splitItem(tok)
		if !sawURL && looksLikeURL(tok, isItem) {
			r.URL, sawURL = tok, true
			continue
		}
		if isItem {
			r.Items = append(r.Items, it)
			continue
		}
		// "Header;" sends a header with an empty value.
		if name, ok := strings.CutSuffix(tok, ";"); ok && isHeaderName(name) {
			r.Items = append(r.Items, Item{Kind: KindHeader, Name: name, Empty: true})
			continue
		}
		return nil, fmt.Errorf("unrecognized argument %q", tok)
	}
	return r, nil
}

func (r *Request) option(args []string, i *int, tok string) error {
	name, value := tok, ""
	hasValue := false
	if j := strings.IndexByte(tok, '='); j > 0 {
		name, value, hasValue = tok[:j], tok[j+1:], true
	}
	need := func() (string, error) {
		if hasValue {
			return value, nil
		}
		if *i+1 >= len(args) {
			return "", fmt.Errorf("option %s requires a value", name)
		}
		*i++
		return args[*i], nil
	}

	var err error
	switch name {
	case "-f", "--form":
		r.Mode = ModeForm
	case "--multipart":
		r.Mode = ModeMultipart
	case "-j", "--json":
		r.Mode, r.acceptJSON = ModeJSON, true
	case "-v", "--verbose":
		r.Verbose = true
	case "--follow":
		r.Follow = true
	case "--path-as-is":
		r.PathAsIs = true
	case "--stream":
		r.Stream = true
	case "--insecure":
		r.Insecure = true
	case "--timeout":
		r.Timeout, err = need()
	case "--max-redirects":
		r.MaxRedirects, err = need()
	case "--proxy":
		r.Proxy, err = need()
	case "-a", "--auth":
		r.Auth, err = need()
	case "--bearer":
		r.Bearer, err = need()
	case "-o", "--output":
		r.Output, err = need()
	case "-b", "--body", "--raw":
		r.RawBody, r.HasRawBody = "", true
		r.RawBody, err = need()
	case "--verify":
		var v string
		if v, err = need(); err == nil {
			switch v {
			case "no":
				r.Insecure = true
			case "yes":
			default:
				err = fmt.Errorf("--verify expects yes or no, got %q", v)
			}
		}
	case "--default-scheme":
		var v string
		if v, err = need(); err == nil {
			r.Scheme = strings.TrimSuffix(v, ":/")
		}
	default:
		// An HTTPie option curl cannot express, or one we do not know yet.
		rec := tok
		if !hasValue && unsupportedTakesValue[name] && *i+1 < len(args) && !strings.HasPrefix(args[*i+1], "-") {
			// Consume the value too, or it would be mistaken for the URL.
			*i++
			rec = name + "=" + args[*i]
		}
		r.Unsupported = append(r.Unsupported, rec)
	}
	return err
}

// unsupportedTakesValue lists HTTPie options that have no curl equivalent but do
// take a value, so the parser can consume that value instead of treating it as
// the URL. Options absent from this set are assumed to be boolean switches.
var unsupportedTakesValue = map[string]bool{
	"-p": true, "--print": true, "-P": true, "--history-print": true,
	"--pretty": true, "-s": true, "--style": true, "--format-options": true,
	"--response-charset": true, "--response-mime": true,
	"--session": true, "--session-read-only": true,
	"--cert": true, "--cert-key": true, "--ssl": true, "--ciphers": true,
	"-A": true, "--auth-type": true, "--max-headers": true, "--boundary": true,
}
