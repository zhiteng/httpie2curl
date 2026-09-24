package convert

import (
	"strings"
	"testing"
)

// line converts an HTTPie argument list and renders it as a shell command line,
// which is the tool's actual user-visible output.
func line(t *testing.T, opts Options, args ...string) string {
	t.Helper()
	req, err := Parse(args, opts)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", args, err)
	}
	argv, _, err := Render(req, opts)
	if err != nil {
		t.Fatalf("Render(%q) error: %v", args, err)
	}
	return Command(argv, opts.Multiline)
}

func TestConvert(t *testing.T) {
	http := Options{Command: "http"}
	https := Options{Command: "https"}

	for _, tc := range []struct {
		name string
		opts Options
		args []string
		want string
	}{
		{
			name: "url only",
			opts: http, args: []string{"pie.dev/get"},
			want: "curl http://pie.dev/get",
		},
		{
			name: "https program selects https",
			opts: https, args: []string{"example.org"},
			want: "curl https://example.org",
		},
		{
			name: "pasted :// keeps the default scheme",
			opts: http, args: []string{"://example.org"},
			want: "curl http://example.org",
		},
		{
			name: "explicit method",
			opts: http, args: []string{"DELETE", "pie.dev/delete"},
			want: "curl -X DELETE http://pie.dev/delete",
		},
		{
			name: "data field implies POST and JSON",
			opts: http, args: []string{"POST", "pie.dev/post", "name=John"},
			want: `curl -X POST -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"name":"John"}' http://pie.dev/post`,
		},
		{
			name: "GET with a body keeps -X GET",
			opts: http, args: []string{"GET", "pie.dev/get", "hello=world"},
			want: `curl -X GET -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"hello":"world"}' http://pie.dev/get`,
		},
		{
			name: "raw json and query parameter",
			opts: http, args: []string{"PUT", "pie.dev/put", "name=John", "age:=29", "token==secret"},
			want: `curl -X PUT -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"name":"John","age":29}' 'http://pie.dev/put?token=secret'`,
		},
		{
			name: "implied method POST for data",
			opts: http, args: []string{"pie.dev/post", "name=John", "email=john@example.org"},
			want: `curl -X POST -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"name":"John","email":"john@example.org"}' http://pie.dev/post`,
		},
		{
			name: "form encoding",
			opts: http, args: []string{"-f", "POST", "pie.dev/post", "name=John Smith"},
			want: `curl -X POST -H 'Content-Type: application/x-www-form-urlencoded; charset=utf-8' -d name=John+Smith http://pie.dev/post`,
		},
		{
			name: "file upload switches to multipart",
			opts: http, args: []string{"-f", "POST", "pie.dev/post", "name=John Smith", "cv@~/files/data.xml"},
			want: `curl -X POST -F 'name=John Smith' -F 'cv=@~/files/data.xml' http://pie.dev/post`,
		},
		{
			name: "multipart without files",
			opts: http, args: []string{"--multipart", "example.org", "hello=world"},
			want: `curl -X POST -F hello=world http://example.org`,
		},
		{
			name: "header unset",
			opts: http, args: []string{"pie.dev/headers", "Accept:", "User-Agent:"},
			want: `curl -H Accept: -H User-Agent: http://pie.dev/headers`,
		},
		{
			name: "empty header uses the semicolon form",
			opts: http, args: []string{"pie.dev/headers", "Header;"},
			want: `curl -H 'Header;' http://pie.dev/headers`,
		},
		{
			name: "escaped separator is a data field",
			opts: http, args: []string{"pie.dev/post", `foo\==bar`},
			want: `curl -X POST -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"foo=":"bar"}' http://pie.dev/post`,
		},
		{
			name: "raw body",
			opts: http, args: []string{"--raw", `{"a":1}`, "pie.dev/post"},
			want: `curl -X POST -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"a":1}' http://pie.dev/post`,
		},
		{
			name: "basic auth",
			opts: http, args: []string{"--auth=user:pass", "pie.dev"},
			want: `curl -u user:pass http://pie.dev`,
		},
		{
			name: "bearer token",
			opts: http, args: []string{"--bearer", "TOKEN", "pie.dev"},
			want: `curl -H 'Authorization: Bearer TOKEN' http://pie.dev`,
		},
		{
			name: "transport flags",
			opts: http, args: []string{"--verify=no", "--timeout=2.5", "--follow", "--max-redirects=3", "pie.dev"},
			want: `curl -L --max-redirs 3 -k --max-time 2.5 http://pie.dev`,
		},
		{
			name: "host:port is a url, not a header",
			opts: http, args: []string{"localhost:8000/api", "-v"},
			want: `curl -v http://localhost:8000/api`,
		},
		{
			name: "header value with a space",
			opts: http, args: []string{"pie.dev/post", "X-Custom:value with space"},
			want: `curl -H 'X-Custom: value with space' http://pie.dev/post`,
		},
		{
			name: "explicit json sets accept",
			opts: http, args: []string{"--json", "pie.dev"},
			want: `curl -H 'Accept: application/json, */*;q=0.5' http://pie.dev`,
		},
		{
			name: "items after -- may start with a dash",
			opts: http, args: []string{"pie.dev/post", "--", "-name-starting-with-dash=foo"},
			want: `curl -X POST -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"-name-starting-with-dash":"foo"}' http://pie.dev/post`,
		},
		{
			name: "full url with query already present",
			opts: http, args: []string{"https://api.github.com/search/repositories", "q==httpie", "per_page==1"},
			want: `curl 'https://api.github.com/search/repositories?q=httpie&per_page=1'`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := line(t, tc.opts, tc.args...); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestUnsupportedOptionIsRecorded(t *testing.T) {
	req, err := Parse([]string{"--session=dev", "pie.dev"}, Options{Command: "http"})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(req.Unsupported) != 1 || req.Unsupported[0] != "--session=dev" {
		t.Fatalf("Unsupported = %v, want [--session=dev]", req.Unsupported)
	}
}

func TestInvalidRawJSONIsRejected(t *testing.T) {
	req, err := Parse([]string{"pie.dev/post", "n:=not-json"}, Options{Command: "http"})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if _, _, err := Render(req, Options{Command: "http"}); err == nil {
		t.Fatal("Render accepted invalid raw JSON")
	}
}

func TestQuote(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "''"},
		{"plain-1.2:3", "plain-1.2:3"},
		{"a b", "'a b'"},
		{"it's", `'it'\''s'`},
		{"*/*;q=0.5", "'*/*;q=0.5'"},
		// A leading tilde must be quoted, or the shell (not curl) expands it.
		{"cv=@~/f.bin", "'cv=@~/f.bin'"},
	} {
		if got := Quote(tc.in); got != tc.want {
			t.Errorf("Quote(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestMultiline(t *testing.T) {
	short := []string{"curl", "-X", "POST", "http://pie.dev"}
	if got, want := Command(short, true), "curl -X POST http://pie.dev"; got != want {
		t.Errorf("short = %q, want %q", got, want)
	}

	body := strings.Repeat("x", 40)
	long := []string{"curl", "-H", "Content-Type: application/json", "-d", body, "http://pie.dev/post"}
	want := "curl -H 'Content-Type: application/json' -d \\\n  " + body + " \\\n  http://pie.dev/post"
	if got := Command(long, true); got != want {
		t.Errorf("long = %q, want %q", got, want)
	}
}
