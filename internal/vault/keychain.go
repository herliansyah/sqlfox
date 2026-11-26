package vault

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const keychainService = "sqlfox-vault"
const keychainAccount = "master-key"

// ReadKeychainMasterPassword attempts to read the master password from the OS keychain
// using native OS CLI tools (CGO-free). Returns ("", false) when unavailable.
func ReadKeychainMasterPassword() (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "security", "find-generic-password", "-s", keychainService, "-a", keychainAccount, "-w")
	case "linux":
		cmd = exec.CommandContext(ctx, "secret-tool", "lookup", "service", keychainService, "account", keychainAccount)
	default:
		return "", false
	}
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	secret := strings.TrimSpace(string(out))
	if secret == "" {
		return "", false
	}
	return secret, true
}

// WriteKeychainMasterPassword stores the master password in the OS keychain using native OS CLI tools.
func WriteKeychainMasterPassword(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// Pass the secret via `security -i` stdin so it never appears in process argv (`ps`).
		cleanSecret := strings.NewReplacer("\n", "", "\r", "").Replace(secret)
		cmd = exec.CommandContext(ctx, "security", "-i")
		cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %q -a %q -w %q\n", keychainService, keychainAccount, cleanSecret))
	case "linux":
		cmd = exec.CommandContext(ctx, "secret-tool", "store", "--label=SqlFox Vault Master Key", "service", keychainService, "account", keychainAccount)
		cmd.Stdin = strings.NewReader(secret)
	default:
		return exec.ErrNotFound
	}
	return cmd.Run()
}
