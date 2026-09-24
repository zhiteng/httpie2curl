# httpie2curl

**English** · [简体中文](README.zh-CN.md)

Turn an [HTTPie](https://httpie.io/) command into the equivalent `curl` command.

```console
$ httpie2curl http PUT pie.dev/put name=John age:=29 token==secret
curl -X PUT -H 'Content-Type: application/json' -H 'Accept: application/json, */*;q=0.5' -d '{"name":"John","age":29}' 'http://pie.dev/put?token=secret'
```

## Why

HTTPie's request-item grammar is a pleasure to type, but it is not portable: the
command cannot be handed to a colleague, pasted into a runbook, or attached to a
bug report unless the reader also has HTTPie installed. `curl` is the lingua
franca. This tool translates one into the other, so the request can be run on any
host and read before it is sent.

It is a single Go binary with **no third-party dependencies**.

## Install

```console
$ git clone https://github.com/zhiteng/httpie2curl.git
$ cd httpie2curl
$ go build -o httpie2curl .
```

`go.mod` declares Go 1.27. `CGO_ENABLED=0 go build` produces a static binary that
can be copied to any machine with the same OS and architecture.

## Usage

```
httpie2curl 0.1.0 — convert an HTTPie command into the equivalent curl command

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
```

## Examples

HTTPS, a header, no body — nothing is added that curl would not already do:

```console
$ httpie2curl https pie.dev/get 'X-Api-Key:secret'
curl -H 'X-Api-Key: secret' https://pie.dev/get
```

A form field and a file upload — the presence of a file field makes it
`multipart/form-data`, and `-F` takes the path verbatim:

```console
$ httpie2curl http -f POST pie.dev/post 'name=John Smith' 'cv@~/files/data.xml'
curl -X POST -F 'name=John Smith' -F 'cv=@~/files/data.xml' http://pie.dev/post
```

`--h2c-multiline` wraps long commands, keeping each flag on the same line as its
value:

```console
$ httpie2curl --h2c-multiline http pie.dev/post 'Authorization:Bearer TOKEN' \
    'Content-Type:application/json' 'items:=[1,2,3]' 'q==search terms'
curl -X POST -H 'Accept: application/json, */*;q=0.5' \
  -H 'Authorization: Bearer TOKEN' -H 'Content-Type: application/json' \
  -d '{"items":[1,2,3]}' 'http://pie.dev/post?q=search+terms'
```

An HTTPie option that curl cannot express is reported rather than silently
dropped:

```console
$ httpie2curl http --session=dev pie.dev
httpie2curl: curl cannot express: --session=dev
httpie2curl: rerun with --h2c-skip-unsupported to convert anyway
$ echo $?
1
```

## What it converts

### Methods and URLs

- A leading `http` or `https` word is consumed and selects the default scheme,
  mirroring HTTPie's two executables. `https example.org` → `https://example.org`,
  `http example.org` → `http://example.org`.
- The `://` form is completed: `http ://example.org` → `http://example.org`.
- A schemeless `host:port/path` is recognised as the URL (`example.org:8080`),
  while `Name:Value` is a header.
- The method is the explicit positional word if present. Otherwise HTTPie's rule
  is applied: `GET` without a body, `POST` with data.
- `GET` is kept explicit when it carries a body, because curl would otherwise
  switch to `POST` as soon as `-d` appears.

### Request items

| HTTPie item | Meaning | curl |
| --- | --- | --- |
| `Name:Value` | request header | `-H 'Name: Value'` |
| `Name:` | unset a default header | `-H Name:` |
| `Name;` | header with an empty value | `-H 'Name;'` |
| `name==value` | query parameter (URL-encoded) | folded into the URL |
| `name=value` | data field | JSON string, or form field under `-f` |
| `name:=json` | raw JSON field | embedded verbatim after validation |
| `name@file` | file upload (with `-f`/`--multipart`) | `-F 'name=@file'` |
| `name=@file`, `:@`, `==@`, `:=@` | value read from a file | inlined, or `-F 'name=<file'` in multipart |

Backslash escapes are honoured (`foo\==bar` is the data field `foo=`), and items
after `--` may start with a dash.

### Serialization

JSON by default, `application/x-www-form-urlencoded` under `-f`, and
`multipart/form-data` under `--multipart` or whenever a file field is present.
JSON string values are quoted while `:=` values are embedded verbatim after
`json.Valid`, so numbers, booleans, arrays and nested objects keep their types:
`age:=29` stays the number `29`, not the string `"29"`.

### Transport options

| HTTPie | curl |
| --- | --- |
| `-v`, `--verbose` | `-v` |
| `--follow` | `-L` |
| `--max-redirects N` | `--max-redirs N` |
| `--timeout SEC` | `--max-time SEC` |
| `--verify no`, `--insecure` | `-k` |
| `--proxy URL` | `-x URL` |
| `--path-as-is` | `--path-as-is` |
| `--stream` | `-N` |
| `-a`, `--auth USER:PASS` | `-u USER:PASS` |
| `--bearer TOKEN` | `-H 'Authorization: Bearer TOKEN'` |
| `-o`, `--output FILE` | `-o FILE` |
| `-b`, `--body`, `--raw RAW` | `-d RAW` |
| `--default-scheme SCHEME` | scheme used for a schemeless URL |

## What the emitted command actually sends

The point is an equivalent request, so the implicit headers HTTPie adds are
reproduced:

- JSON data → `Content-Type: application/json` **and**
  `Accept: application/json, */*;q=0.5`. The `Accept` matters: curl's own default
  is `*/*`, which would change content negotiation.
- `-f` → `Content-Type: application/x-www-form-urlencoded; charset=utf-8`.
- multipart → no `Content-Type` is emitted, because curl has to own the boundary.
- Without data, HTTPie sends `Accept: */*`, which already matches curl's default,
  so nothing is emitted.

Shell quoting is POSIX. Only this set of characters is left bare:
`A-Za-z0-9` and `_@%+=:,./-`. Everything else is single-quoted — including `~`,
`?`, `&` and `*`, so a `~` inside a value is never expanded by the shell and a
query string can never background the command:

```console
$ httpie2curl http pie.dev/search q==httpie
curl 'http://pie.dev/search?q=httpie'
```

## Options curl cannot express

HTTPie options with no curl equivalent (`--session`, `--print`/`--pretty`,
`--style`, `--offline`, `--cert`, `-A`/`--auth-type=digest`, `--check-status`,
plugins, …) are collected and reported. **By default that is an error** (`1`), and
`--h2c-skip-unsupported` downgrades it to a warning, so a partial conversion
cannot pass unnoticed.

For the options that HTTPie gives a value to, the value is consumed together with
the flag, so it can never be mistaken for the URL:

```console
$ httpie2curl http --style solarized pie.dev
httpie2curl: curl cannot express: --style=solarized
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | converted (possibly with notes on stderr) |
| `1` | conversion failed: unrecognized argument, invalid raw JSON, unreadable file, no URL in the request, or an inexpressible HTTPie option without `--h2c-skip-unsupported` |
| `2` | usage error: unknown `--h2c-*` option, or no HTTPie arguments at all — a bare `httpie2curl`, or only a program name such as `httpie2curl http` |

## Known limitations

- **Batch conversion is not implemented.** Converting the `http …` lines in a
  script or a shell history file is planned.
- **The reverse direction (`curl` → HTTPie) is not implemented.** curl's
  `-d`/`--data-binary`/`--data-urlencode`/`-G`/`-F`/`--json` overlap is
  many-to-one, so that conversion is lossy and needs an explicit uncertainty
  report instead of pretending to be exact.
- **File-based items are read at conversion time**, so the tool touches the
  filesystem, expands `~` itself, and strips trailing CR/LF. For `name@file`
  uploads the path is handed to curl as `-F 'name=@file'` and **curl does not
  expand `~`**, so `cv@~/x` stays literal.
- `:=` has no form or multipart representation: under `-f`/`--multipart` it is
  reported as a note and dropped.
- An explicit `Content-Type` combined with multipart is not handled (curl must
  own the boundary); it is reported.
- URL-versus-header disambiguation is a heuristic. A bare word is the URL; an
  all-uppercase bare word before the URL is read as a method (so `http AHOY
  pie.dev` works, but a schemeless uppercase hostname would not).
- Quoting is POSIX only; the output is not PowerShell-safe.
- HTTPie's `User-Agent: HTTPie/<version>` is never reproduced — curl sends its
  own. `--h2c-with-defaults` covers `Accept-Encoding` by adding `--compressed`.
- An unknown HTTPie option is assumed to take no value unless it appears in the
  known value-taking list.

## Development

```console
$ go test ./...     # table-driven conversion tests, quoting, and error paths
$ gofmt -l .
$ go vet ./...
```

- `main.go` — the CLI: `--h2c-*` parsing, the program-name word, output, exit codes.
- `internal/convert/item.go` — request-item grammar: separators, escaping,
  URL-versus-header disambiguation.
- `internal/convert/parse.go` — HTTPie argv → `Request`.
- `internal/convert/render.go` — `Request` → curl argv, including the implicit
  `Content-Type`/`Accept` and the JSON/form/multipart bodies.
- `internal/convert/shell.go` — POSIX quoting and multi-line output.

The grammar is checked against the HTTPie 3.2.4 CLI documentation. Conversions are
additionally verified end to end by executing the generated command against a
local echo server and comparing the received method, path, headers and body.

## Status

Initial version (`0.1.0`): forward conversion is complete for the surface listed
above. Batch conversion, the reverse direction, and a LICENSE are not done yet.
