package ui

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var secretPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
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
