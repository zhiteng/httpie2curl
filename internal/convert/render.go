package convert

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Render turns a parsed request into a curl argv plus advisory notes.
func Render(r *Request, opts Options) ([]string, []string, error) {
	var notes []string

	// A file upload forces multipart/form-data, even under --form.
	mode := r.Mode
	for _, it := range r.Items {
		if it.Kind == KindFile {
			mode = ModeMultipart
		}
	}
	hasData := r.HasRawBody
	for _, it := range r.Items {
		switch it.Kind {
		case KindData, KindRawJSON, KindFile:
			hasData = true
		}
	}
	method := r.Method
	if method == "" {
		method = "GET"
		if hasData {
			method = "POST"
		}
	}

	curl := opts.CurlBinary
	if curl == "" {
		curl = "curl"
	}
	argv := []string{curl}

	if r.Verbose {
		argv = append(argv, "-v")
	}
	if r.Follow {
		argv = append(argv, "-L")
	}
	if r.MaxRedirects != "" {
		argv = append(argv, "--max-redirs", r.MaxRedirects)
	}
	if r.Insecure {
		argv = append(argv, "-k")
	}
	if r.Proxy != "" {
		argv = append(argv, "-x", r.Proxy)
	}
	if r.Timeout != "" {
		argv = append(argv, "--max-time", r.Timeout)
	}
	if r.PathAsIs {
		argv = append(argv, "--path-as-is")
	}
	if r.Stream {
		argv = append(argv, "-N")
	}
	if opts.WithDefaults {
		// HTTPie asks for gzip/deflate and decodes the response itself; curl
		// only does both with --compressed.
		argv = append(argv, "--compressed")
	}
	if r.Output != "" {
		argv = append(argv, "-o", r.Output)
	}
	if r.Auth != "" {
		argv = append(argv, "-u", r.Auth)
	}
	if r.Bearer != "" {
		argv = append(argv, "-H", "Authorization: Bearer "+r.Bearer)
	}
	// curl assumes GET without a body and POST with one, so -X is only needed
	// when the method differs, or when GET carries a body (HTTPie allows that).
	if method != "GET" || hasData {
		argv = append(argv, "-X", method)
	}

	// The Content-Type HTTPie would add for the body we are about to generate.
	if hasData && !r.hasHeader("Content-Type") {
		switch mode {
		case ModeMultipart:
			// curl derives multipart/form-data (with its boundary) from -F.
		case ModeForm:
			argv = append(argv, "-H", "Content-Type: application/x-www-form-urlencoded; charset=utf-8")
		default:
			argv = append(argv, "-H", "Content-Type: application/json")
		}
	}
	// HTTPie sets Accept alongside Content-Type whenever it serializes data as
	// JSON, so the converted command must set it too: curl would otherwise send
	// its own "Accept: */*". Without data HTTPie also sends "Accept: */*", which
	// already matches curl's default, so nothing is emitted.
	if r.acceptJSON || (mode == ModeJSON && hasData) {
		argv = append(argv, "-H", "Accept: application/json, */*;q=0.5")
	}

	// Headers render in place; body items are collected to stay in input order.
	var fields []Item
	for _, it := range r.Items {
		switch it.Kind {
		case KindHeader:
			switch {
			case it.Unset:
				argv = append(argv, "-H", it.Name+":")
			case it.Empty:
				argv = append(argv, "-H", it.Name+";")
			default:
				v := it.Value
				if it.FromFile {
					data, err := readValue(v)
					if err != nil {
						return nil, nil, err
					}
					v = data
				}
				argv = append(argv, "-H", it.Name+": "+v)
			}
		case KindQuery:
			// Query parameters are folded into the URL by requestURL.
		default:
			fields = append(fields, it)
		}
	}

	switch {
	case r.HasRawBody:
		argv = append(argv, "-d", r.RawBody)
	case mode == ModeMultipart:
		for _, it := range fields {
			switch {
			case it.Kind == KindRawJSON:
				notes = append(notes, fmt.Sprintf("raw JSON field %q is not representable in multipart mode", it.Name))
			case it.Kind == KindFile:
				// The "@" separator is consumed while parsing, so put it back.
				argv = append(argv, "-F", it.Name+"=@"+it.Value)
			case it.FromFile:
				argv = append(argv, "-F", it.Name+"=<"+it.Value)
			default:
				argv = append(argv, "-F", it.Name+"="+it.Value)
			}
		}
	case mode == ModeForm:
		var parts []string
		for _, it := range fields {
			if it.Kind == KindRawJSON {
				notes = append(notes, fmt.Sprintf("raw JSON field %q is not representable in form mode", it.Name))
				continue
			}
			v := it.Value
			if it.FromFile {
				data, err := readValue(v)
				if err != nil {
					return nil, nil, err
				}
				v = data
			}
			parts = append(parts, formEncode(it.Name, v))
		}
		if len(parts) > 0 {
			argv = append(argv, "-d", strings.Join(parts, "&"))
		}
	default:
		if len(fields) > 0 {
			body, err := jsonBody(fields)
			if err != nil {
				return nil, nil, err
			}
			argv = append(argv, "-d", body)
		}
	}

	target, err := requestURL(r)
	if err != nil {
		return nil, nil, err
	}
	return append(argv, target), notes, nil
}

func (r *Request) hasHeader(name string) bool {
	for _, it := range r.Items {
		if it.Kind == KindHeader && strings.EqualFold(it.Name, name) {
			return true
		}
	}
	return false
}

// jsonBody builds the JSON object HTTPie would send, in field order. String
// values are quoted; ":=" fields are embedded verbatim after validation.
func jsonBody(items []Item) (string, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, it := range items {
		v := it.Value
		if it.FromFile {
			data, err := readValue(v)
			if err != nil {
				return "", err
			}
			v = data
		}
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(it.Name)
		if err != nil {
			return "", err
		}
		b.Write(key)
		b.WriteByte(':')
		if it.Kind == KindRawJSON {
			if !json.Valid([]byte(v)) {
				return "", fmt.Errorf("field %q is not valid JSON: %s", it.Name, v)
			}
			b.WriteString(v)
			continue
		}
		val, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		b.Write(val)
	}
	b.WriteByte('}')
	return b.String(), nil
}

func requestURL(r *Request) (string, error) {
	if r.URL == "" {
		return "", fmt.Errorf("no URL given")
	}
	u := r.URL
	if !strings.Contains(u, "://") {
		u = r.Scheme + "://" + strings.TrimPrefix(u, "://")
	}
	var params []string
	for _, it := range r.Items {
		if it.Kind != KindQuery {
			continue
		}
		v := it.Value
		if it.FromFile {
			data, err := readValue(v)
			if err != nil {
				return "", err
			}
			v = data
		}
		params = append(params, url.QueryEscape(it.Name)+"="+url.QueryEscape(v))
	}
	if len(params) == 0 {
		return u, nil
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + strings.Join(params, "&"), nil
}

func formEncode(name, value string) string {
	return url.QueryEscape(name) + "=" + url.QueryEscape(value)
}

func readValue(path string) (string, error) {
	b, err := os.ReadFile(expandHome(path))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	// HTTPie strips the trailing newline of file-based values.
	return strings.TrimRight(string(b), "\r\n"), nil
}

func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
