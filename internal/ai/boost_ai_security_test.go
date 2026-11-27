package ai

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herliansyah/sqlfox/internal/netguard"
)

func TestBoostP05_SSRFBlocksLoopbackAndRemoteOllama(t *testing.T) {
	// Loopback IPs must be blocked when allowLoopback is false (prevents DNS rebinding to 127.0.0.1 / ::1).
	for _, raw := range []string{"127.0.0.1", "127.0.1.1", "::1", "169.254.169.254", "100.64.0.1", "fd00:ec2::254", "0.0.0.0"} {
		ip := net.ParseIP(raw)
		if !netguard.IsBlockedDialIP(ip, false, false) {
			t.Errorf("IsBlockedDialIP(%s, false, false) = false; want true", raw)
		}
	}
	if netguard.IsBlockedDialIP(net.ParseIP("127.0.0.1"), true, false) {
		t.Errorf("expected explicit loopback to be allowed when allowLoopback=true")
	}
	if !netguard.IsBlockedDialIP(net.ParseIP("10.0.0.5"), true, false) {
		t.Errorf("expected RFC1918 private IP to be blocked when allowPrivate=false")
	}

	// Remote requests MUST block Ollama dialing localhost.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer ts.Close()

	if DetectOllama(context.Background(), ts.URL, true) {
		t.Fatalf("DetectOllama with isRemote=true should block loopback endpoint %s", ts.URL)
	}
	client := NewAIClient(ClientConfig{
		Provider:      ProviderOllama,
		Endpoint:      ts.URL,
		Model:         "llama3",
		RemoteRequest: true,
	})
	if _, err := client.Complete(context.Background(), CompletionRequest{UserPrompt: "hi"}); err == nil {
		t.Fatalf("remote AI client should block dialing loopback Ollama endpoint")
	}
}

func TestBoostP1_RedactSensitiveLiterals(t *testing.T) {
	in := `SELECT * FROM users WHERE email = 'ceo@corp.com'; DETAIL: Key (email)=(ceo@corp.com) already exists.`
	out := RedactSensitiveLiterals(in)
	if strings.Contains(out, "ceo@corp.com") {
		t.Fatalf("expected sensitive literal/PII to be redacted, got: %s", out)
	}
}
