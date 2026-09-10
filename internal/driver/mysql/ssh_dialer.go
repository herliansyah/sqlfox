package mysql

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshTunnel struct {
	client *ssh.Client
}

func createSSHTunnel(ctx context.Context, host string, port int, user, password, keyPathOrContent string) (*sshTunnel, error) {
	if port <= 0 {
		port = 22
	}

	var authMethods []ssh.AuthMethod
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}

	if keyPathOrContent != "" {
		keyBytes := []byte(keyPathOrContent)
		if _, err := os.Stat(keyPathOrContent); err == nil {
			if content, err := os.ReadFile(keyPathOrContent); err == nil {
				keyBytes = content
			}
		}

		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication method provided (password or private key required)")
	}

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // ponytail: SSH host key verification skipped for local developer flexibility; upgrade path: parse ~/.ssh/known_hosts
		Timeout:         10 * time.Second,
	}

	sshAddr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", sshAddr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH dial failed to %s: %w", sshAddr, err)
	}

	return &sshTunnel{client: client}, nil
}

func (t *sshTunnel) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	return t.client.Dial("tcp", addr)
}

func (t *sshTunnel) Close() error {
	if t.client != nil {
		return t.client.Close()
	}
	return nil
}
