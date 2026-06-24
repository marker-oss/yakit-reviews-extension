package installer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHExecutor struct {
	client       *ssh.Client
	user         string
	sudoPassword string
}

func NewSSHExecutor(cfg Config) (*SSHExecutor, error) {
	auth, err := sshAuthMethod(cfg.Server)
	if err != nil {
		return nil, err
	}
	client, err := ssh.Dial("tcp", cfg.SSHAddress(), &ssh.ClientConfig{
		User:            cfg.Server.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &SSHExecutor{
		client:       client,
		user:         cfg.Server.User,
		sudoPassword: cfg.EffectiveSudoPassword(),
	}, nil
}

func sshAuthMethod(cfg ServerConfig) (ssh.AuthMethod, error) {
	switch cfg.AuthMethod {
	case SSHAuthPassword:
		return ssh.Password(cfg.Password), nil
	case SSHAuthKey:
		key, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, err
		}
		var signer ssh.Signer
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, err
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("unsupported SSH auth method %q", cfg.AuthMethod)
	}
}

func (e *SSHExecutor) Close() error {
	if e.client == nil {
		return nil
	}
	return e.client.Close()
}

func (e *SSHExecutor) Run(ctx context.Context, command string, sudo bool) (string, error) {
	if sudo && e.user != "root" {
		command = "sudo -S -p '' sh -c " + shellQuote(command)
	}
	session, err := e.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if sudo && e.user != "root" {
		session.Stdin = bytes.NewBufferString(e.sudoPassword + "\n")
	}

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return stdout.String() + stderr.String(), ctx.Err()
	case err := <-done:
		output := stdout.String() + stderr.String()
		if err != nil {
			return output, fmt.Errorf("%w: %s", err, output)
		}
		return output, nil
	}
}

func (e *SSHExecutor) WriteFile(ctx context.Context, path, content string, mode string, sudo bool) error {
	tmp := "/tmp/reviews-installer-file"
	writeCmd := "cat > " + shellQuote(tmp)
	session, err := e.client.NewSession()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	session.Stdin = bytes.NewBufferString(content)
	session.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- session.Run(writeCmd) }()
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		return ctx.Err()
	case err := <-done:
		_ = session.Close()
		if err != nil {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
	}
	installCmd := fmt.Sprintf("install -m %s %s %s && rm -f %s", shellQuote(mode), shellQuote(tmp), shellQuote(path), shellQuote(tmp))
	_, err = e.Run(ctx, installCmd, sudo)
	return err
}
