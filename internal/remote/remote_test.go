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

func TestSSHArgsIncludeIdentityFile(t *testing.T) {
	client := Client{
		Target: config.Target{
			Host: "example.com",
			User: "deploy",
			Port: 2222,
		},
		Auth: Auth{IdentityFile: "/home/me/.ssh/id_ed25519"},
	}

	got := client.sshArgs("true")
	want := []string{
		"-p", "2222",
		"-i", "/home/me/.ssh/id_ed25519",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"deploy@example.com",
		"true",
	}
	if len(got) != len(want) {
		t.Fatalf("sshArgs length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sshArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSCPArgsIncludeIdentityFile(t *testing.T) {
	client := Client{
		Target: config.Target{
			Host: "example.com",
			User: "deploy",
			Port: 2222,
		},
		Auth: Auth{IdentityFile: "/home/me/.ssh/id_ed25519"},
	}

	got := client.scpArgs("bundle.tar.gz", "/opt/my-app/incoming/bundle.tar.gz")
	want := []string{
		"-P", "2222",
		"-i", "/home/me/.ssh/id_ed25519",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"bundle.tar.gz",
		"deploy@example.com:/opt/my-app/incoming/bundle.tar.gz",
	}
	if len(got) != len(want) {
		t.Fatalf("scpArgs length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scpArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestComposeCommandArgsUsesCurrentComposeAndEnvFile(t *testing.T) {
	client := Client{ComposeEnvFile: "shared/.env"}
	got := client.composeCommandArgs("up -d")
	want := "--env-file current/.tarsail.env --env-file 'shared/.env' -f current/compose.yaml up -d"
	if got != want {
		t.Fatalf("composeCommandArgs() = %q, want %q", got, want)
	}
}

func TestUniqueStrings(t *testing.T) {
	got := uniqueStrings([]string{"shared/.env", "shared/.env", "", "shared/auth/htpasswd"})
	want := []string{"shared/.env", "shared/auth/htpasswd"}
	if len(got) != len(want) {
		t.Fatalf("uniqueStrings length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("uniqueStrings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
