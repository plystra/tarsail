package ui

import (
	"bytes"
	"testing"
)

func TestRedactingWriterRedactsSecretLikeOutput(t *testing.T) {
	var out bytes.Buffer
	writer := RedactingWriter{Writer: &out}
	if _, err := writer.Write([]byte("DATABASE_PASSWORD=secret-value\n")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "DATABASE_PASSWORD=[redacted]\n" {
		t.Fatalf("redacted output = %q", got)
	}
}

func TestRedactHidesInfrastructureUrls(t *testing.T) {
	got := Redact("Database: jdbc:postgresql://db.example.com:5432/app?sslmode=prefer")
	want := "Database: [redacted-url]"
	if got != want {
		t.Fatalf("Redact() = %q, want %q", got, want)
	}
}

func TestPromptPasswordReadsLineWithoutEchoRequirement(t *testing.T) {
	var out bytes.Buffer
	password, err := PromptPassword(bytes.NewBufferString("secret-value\n"), &out, "Password: ")
	if err != nil {
		t.Fatal(err)
	}
	if password != "secret-value" {
		t.Fatalf("password = %q", password)
	}
	if out.String() != "Password: " {
		t.Fatalf("prompt output = %q", out.String())
	}
}
