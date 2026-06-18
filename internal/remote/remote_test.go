package remote

import "testing"

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"":            "''",
		"/opt/my-app": "'/opt/my-app'",
		"it's":        "'it'\"'\"'s'",
	}
	for input, want := range tests {
		if got := ShellQuote(input); got != want {
			t.Fatalf("ShellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}
