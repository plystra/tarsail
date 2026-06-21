package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/plystra/tarsail/internal/ui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func (c Client) passwordCapture(ctx context.Context, area, script string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := c.passwordRun(ctx, area, script, &stdout, &stderr); err != nil {
		return stdout.String(), c.commandError(area, script, stdout.String(), stderr.String(), err)
	}
	return stdout.String(), nil
}

func (c Client) passwordStream(ctx context.Context, area, script string) error {
	stdout := ui.RedactingWriter{Writer: c.Stdout}
	stderr := ui.RedactingWriter{Writer: c.Stderr}
	if err := c.passwordRun(ctx, area, script, stdout, stderr); err != nil {
		return c.commandError(area, script, "", "", err)
	}
	return nil
}

func (c Client) passwordRun(ctx context.Context, area, script string, stdout, stderr io.Writer) error {
	client, err := c.passwordSSHClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("[%s] Could not create SSH session: %w", area, err)
	}
	defer session.Close()

	if stdout != nil {
		session.Stdout = stdout
	}
	if stderr != nil {
		session.Stderr = stderr
	}
	return session.Run("sh -c " + ShellQuote(script))
}

func (c Client) passwordUpload(ctx context.Context, area, localPath, remotePath string) error {
	client, err := c.passwordSSHClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("[%s] Could not open local file for upload: %w", area, err)
	}
	defer file.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("[%s] Could not create SSH session: %w", area, err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("[%s] Could not open SSH stdin: %w", area, err)
	}

	command := "cat > " + ShellQuote(remotePath)
	if err := session.Start("sh -c " + ShellQuote(command)); err != nil {
		return fmt.Errorf("[%s] Could not start remote upload to %s:%s: %w", area, c.address(), remotePath, err)
	}
	if _, err := io.Copy(stdin, file); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("[%s] Could not stream upload data to %s:%s: %w", area, c.address(), remotePath, err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("[%s] Could not close remote upload stream: %w", area, err)
	}
	if err := session.Wait(); err != nil {
		detail := strings.TrimSpace(ui.Redact(stderr.String()))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("[%s] Could not upload file to %s:%s.\n\nDetails:\n  %s", area, c.address(), remotePath, strings.ReplaceAll(detail, "\n", "\n  "))
	}
	return nil
}

func (c Client) passwordSSHClient(ctx context.Context) (*ssh.Client, error) {
	hostKeyCallback, err := knownHostsCallback()
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            c.Target.User,
		Auth:            []ssh.AuthMethod{ssh.Password(c.Auth.Password)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}
	address := net.JoinHostPort(c.Target.Host, strconv.Itoa(c.Target.Port))

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("[remote:ssh] Could not connect to %s: %w", address, err)
	}

	done := make(chan struct{})
	var clientConn ssh.Conn
	var chans <-chan ssh.NewChannel
	var reqs <-chan *ssh.Request
	var handshakeErr error
	go func() {
		clientConn, chans, reqs, handshakeErr = ssh.NewClientConn(conn, address, config)
		close(done)
	}()

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return nil, ctx.Err()
	case <-done:
	}
	if handshakeErr != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("[remote:ssh] Password authentication failed for %s: %w", c.address(), handshakeErr)
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
}

func knownHostsCallback() (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("[remote:ssh] Could not resolve home directory for known_hosts: %w", err)
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("[remote:ssh] Could not load SSH known_hosts file: %s\n\nHow to fix:\n  Connect once with your system ssh command, verify the host key, then rerun Tarsail.", path)
	}
	return callback, nil
}
