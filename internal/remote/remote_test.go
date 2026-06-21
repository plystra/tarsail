package remote

import (
	"testing"

	"github.com/plystra/tarsail/internal/config"
)

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

func TestSCPDestinationDoesNotShellQuoteRemotePath(t *testing.T) {
	client := Client{Target: config.Target{
		Host: "example.com",
		User: "deploy",
	}}

	got := client.scpDestination("/opt/my-app/incoming/release.tar.gz")
	want := "deploy@example.com:/opt/my-app/incoming/release.tar.gz"
	if got != want {
		t.Fatalf("scpDestination() = %q, want %q", got, want)
	}
}
