//go:build sshd

package sshd

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// RunCommand executes a command over SSH and returns stdout, stderr, and exit code.
func RunCommand(t *testing.T, client *ssh.Client, cmd string) (stdout, stderr string, exitCode int) {
	t.Helper()
	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("creating SSH session: %v", err)
	}
	defer sess.Close()

	var outBuf, errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf

	err = sess.Run(cmd)
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			t.Fatalf("running command %q: %v", cmd, err)
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// runCommandE is like RunCommand but returns an error instead of calling t.Fatal.
// Safe to call from goroutines.
func runCommandE(client *ssh.Client, cmd string) (stdout, stderr string, exitCode int, err error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", "", 0, fmt.Errorf("creating session: %w", err)
	}
	defer sess.Close()

	var outBuf, errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf

	err = sess.Run(cmd)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return outBuf.String(), errBuf.String(), exitErr.ExitStatus(), nil
		}
		return "", "", 0, fmt.Errorf("running %q: %w", cmd, err)
	}
	return outBuf.String(), errBuf.String(), 0, nil
}

// SFTPClient opens an SFTP session over the SSH connection.
func SFTPClient(t *testing.T, client *ssh.Client) *sftp.Client {
	t.Helper()
	c, err := sftp.NewClient(client)
	if err != nil {
		t.Fatalf("opening SFTP session: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// SCPUpload sends file contents to a remote path via SSH exec of "cat > path".
func SCPUpload(t *testing.T, client *ssh.Client, remotePath string, contents []byte) {
	t.Helper()
	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("creating SSH session for scp upload: %v", err)
	}
	defer sess.Close()

	sess.Stdin = bytes.NewReader(contents)
	var errBuf bytes.Buffer
	sess.Stderr = &errBuf

	cmd := fmt.Sprintf("cat > '%s'", remotePath)
	if err := sess.Run(cmd); err != nil {
		t.Fatalf("scp upload to %s: %v (stderr: %s)", remotePath, err, errBuf.String())
	}
}

// SCPDownload reads file contents from a remote path via SSH exec of "cat path".
func SCPDownload(t *testing.T, client *ssh.Client, remotePath string) []byte {
	t.Helper()
	stdout, _, exitCode := RunCommand(t, client, fmt.Sprintf("cat '%s'", remotePath))
	if exitCode != 0 {
		t.Fatalf("scp download from %s: exit %d", remotePath, exitCode)
	}
	return []byte(stdout)
}

// ShellSession wraps an interactive PTY session for testing.
type ShellSession struct {
	Session *ssh.Session
	Stdin   io.WriteCloser
	Stdout  *bytes.Buffer
}

// InteractiveSession opens an SSH session with PTY allocation.
func InteractiveSession(t *testing.T, client *ssh.Client) *ShellSession {
	t.Helper()
	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("creating interactive session: %v", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm", 40, 80, modes); err != nil {
		sess.Close()
		t.Fatalf("requesting PTY: %v", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		t.Fatalf("getting stdin pipe: %v", err)
	}

	var stdout bytes.Buffer
	sess.Stdout = &stdout

	if err := sess.Shell(); err != nil {
		sess.Close()
		t.Fatalf("starting shell: %v", err)
	}

	t.Cleanup(func() {
		stdin.Close()
		sess.Close()
	})

	return &ShellSession{
		Session: sess,
		Stdin:   stdin,
		Stdout:  &stdout,
	}
}
