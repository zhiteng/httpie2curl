// Command httpie2curl converts an HTTPie command line into the equivalent curl
// command line.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/zhiteng/httpie2curl/internal/convert"
)

const version = "0.1.0"

const usage = `httpie2curl ` + version + ` — convert an HTTPie command into the equivalent curl command

usage:
  httpie2curl [--h2c-OPTION...] [http|https] <HTTPie arguments...>

examples:
  httpie2curl http POST pie.dev/post name=John age:=29
  httpie2curl https pie.dev/get 'X-Api-Key:secret' q==httpie
  httpie2curl --h2c-skip-unsupported http --session=dev pie.dev/get

Everything that is not a --h2c-* option is handed to the HTTPie parser, so an
HTTPie command line can be pasted almost verbatim. A leading http/https word is
consumed as HTTPie's program name and selects the default scheme.

tool options:
  --h2c-curl=PATH          curl binary to emit (default "curl")
  --h2c-with-defaults      reproduce HTTPie's transport defaults: add curl
                           --compressed, so gzip/deflate responses are decoded
                           the way HTTPie decodes them
  --h2c-skip-unsupported   warn and continue instead of failing on HTTPie options
                           that curl cannot express
  --h2c-multiline          print the command over multiple lines with backslashes
  --h2c-help               show this help
  --h2c-version            print the version
`

func main() {
	opts := convert.Options{}
	args := os.Args[1:]

	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "--h2c-") {
		a := args[i]
		switch {
		case a == "--h2c-help":
			fmt.Print(usage)
			return
		case a == "--h2c-version":
			fmt.Println("httpie2curl " + version)
			return
		case a == "--h2c-multiline":
			opts.Multiline = true
		case a == "--h2c-with-defaults":
			opts.WithDefaults = true
		case a == "--h2c-skip-unsupported":
			opts.SkipUnsupported = true
		case strings.HasPrefix(a, "--h2c-curl="):
			opts.CurlBinary = strings.TrimPrefix(a, "--h2c-curl=")
		default:
			fmt.Fprintf(os.Stderr, "httpie2curl: unknown option %s\n\n%s", a, usage)
			os.Exit(2)
		}
		i++
	}
	rest := args[i:]

	// A leading program name selects the default scheme, exactly like HTTPie's
	// own http/https pair.
	if len(rest) > 0 && (rest[0] == "http" || rest[0] == "https") {
		opts.Command = rest[0]
		rest = rest[1:]
	}
	if opts.Command == "" {
		opts.Command = "http"
	}
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	req, err := convert.Parse(rest, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "httpie2curl: %v\n", err)
		os.Exit(1)
	}
	if len(req.Unsupported) > 0 && !opts.SkipUnsupported {
		fmt.Fprintf(os.Stderr, "httpie2curl: curl cannot express: %s\n", strings.Join(req.Unsupported, ", "))
		fmt.Fprintln(os.Stderr, "httpie2curl: rerun with --h2c-skip-unsupported to convert anyway")
		os.Exit(1)
	}

	argv, notes, err := convert.Render(req, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "httpie2curl: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(convert.Command(argv, opts.Multiline))
	for _, u := range req.Unsupported {
		fmt.Fprintf(os.Stderr, "httpie2curl: note: dropped %s (no curl equivalent)\n", u)
	}
	for _, n := range notes {
		fmt.Fprintf(os.Stderr, "httpie2curl: note: %s\n", n)
	}
}
