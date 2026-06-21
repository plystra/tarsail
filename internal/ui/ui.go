package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

var secretPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)\b(jdbc:[^\s]+)`), `[redacted-url]`},
	{regexp.MustCompile(`(?i)\b((postgres|postgresql|mysql|mariadb|mongodb|redis)://[^\s]+)`), `[redacted-url]`},
	{regexp.MustCompile(`(?i)(password|passwd|pwd|token|api[_-]?key|secret)=([^\s]+)`), `${1}=[redacted]`},
	{regexp.MustCompile(`(?i)(://[^:\s]+:)([^@\s]+)(@)`), `${1}[redacted]${3}`},
}

func Redact(value string) string {
	redacted := value
	for _, pattern := range secretPatterns {
		redacted = pattern.pattern.ReplaceAllString(redacted, pattern.replacement)
	}
	return redacted
}

type RedactingWriter struct {
	Writer io.Writer
}

func (w RedactingWriter) Write(p []byte) (int, error) {
	if w.Writer == nil {
		return len(p), nil
	}
	if _, err := io.WriteString(w.Writer, Redact(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

func Step(w io.Writer, current, total int, text string) {
	fmt.Fprintf(w, "[%d/%d] %s\n", current, total, text)
}

func OK(w io.Writer, text string) {
	fmt.Fprintf(w, "  OK   %s\n", text)
}

func Fail(w io.Writer, text string) {
	fmt.Fprintf(w, "  FAIL %s\n", text)
}

func PromptYesNo(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprint(out, prompt)
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func PromptPassword(in io.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		password, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(password), "\r\n"), nil
	}
	reader := bufio.NewReader(in)
	password, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(password, "\r\n"), nil
}
