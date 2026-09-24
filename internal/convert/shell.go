package convert

import "strings"

// shellSafe lists the characters that need no quoting in a POSIX shell word.
// Note that "~" is deliberately absent: an unquoted "~" would be expanded by
// the shell, which must not happen inside "--form" values like "cv=@~/f.bin".
const shellSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-"

// Quote returns s quoted for a POSIX shell when it contains anything unsafe.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune(shellSafe, r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Command renders an argv as a shell command line. In multiline mode arguments
// are packed onto the first line while they fit within wrapWidth, then continued
// one per line with backslashes.
const wrapWidth = 72

func Command(argv []string, multiline bool) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = Quote(a)
	}
	if !multiline || len(quoted) == 0 {
		return strings.Join(quoted, " ")
	}

	var b strings.Builder
	b.WriteString(quoted[0])
	line := len(quoted[0])
	rest := quoted[1:]
	for len(rest) > 0 && line+1+len(rest[0]) <= wrapWidth {
		b.WriteByte(' ')
		b.WriteString(rest[0])
		line += 1 + len(rest[0])
		rest = rest[1:]
	}
	for _, q := range rest {
		b.WriteString(" \\\n  ")
		b.WriteString(q)
	}
	return b.String()
}
