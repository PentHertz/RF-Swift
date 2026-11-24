package workbench

import "testing"

func cleanAll(chunks ...string) string {
	var c termCleaner
	out := ""
	for _, ch := range chunks {
		out += string(c.Write([]byte(ch)))
	}
	return out + string(c.Flush())
}

func TestTermCleanerStripsEscapesAndNormalizesLines(t *testing.T) {
	cases := []struct {
		name   string
		chunks []string
		want   string
	}{
		{"crlf", []string{"one\r\ntwo\r\n"}, "one\ntwo\n"},
		{"colour", []string{"\x1b[1;32mOK\x1b[0m done\r\n"}, "OK done\n"},
		{"split escape", []string{"a\x1b[3", "1mb\x1b[0", "m\n"}, "ab\n"},
		{"progress overwrite", []string{"10%\r20%\r100%\r\n"}, "100%\n"},
		{"erase line", []string{"old text\r\x1b[Knew\n"}, "new\n"},
		{"osc title", []string{"\x1b]0;title\x07shell$ "}, "shell$ "},
		{"backspace utf8", []string{"caf\xc3\xa9\b\be\n"}, "cae\n"},
		{"double cr before lf", []string{"line\r\r\n"}, "line\n"},
		{"bell and nul dropped", []string{"a\x07b\x00c\n"}, "abc\n"},
		{"charset designation", []string{"\x1b(Bplain\n"}, "plain\n"},
	}
	for _, tc := range cases {
		if got := cleanAll(tc.chunks...); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTermCleanerKeepsPromptAsPartialLine(t *testing.T) {
	var c termCleaner
	if out := c.Write([]byte("ready\r\npm3 --> ")); string(out) != "ready\n" {
		t.Fatalf("committed %q", out)
	}
	if p := string(c.Partial()); p != "pm3 --> " {
		t.Fatalf("partial %q", p)
	}
	if f := string(c.Flush()); f != "pm3 --> " || len(c.Partial()) != 0 {
		t.Fatalf("flush %q", f)
	}
}

func TestTrimToRunesDropsCutSequences(t *testing.T) {
	if got := trimToRunes("ab\xc3", true); got != "ab" {
		t.Fatalf("end cut: %q", got)
	}
	if got := trimToRunes("\xa9cd", false); got != "cd" {
		t.Fatalf("start cut: %q", got)
	}
	if got := trimToRunes("caf\xc3\xa9", true); got != "caf\xc3\xa9" {
		t.Fatalf("complete rune trimmed: %q", got)
	}
}
