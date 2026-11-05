//go:build ignore

// sign-manifest.go is the offline release-signing helper for SqlFox's self-update trust anchor.
// It is NOT part of the shipped binary (build tag `ignore`); run it with `go run`.
//
// It uses only the Go standard library (crypto/ed25519), honoring the CGO-Free Invariant.
//
// Generate a keypair (do this once; keep the private key offline / in a CI secret):
//
//	go run scripts/sign-manifest.go -genkey
//	  -> prints PUBLIC_KEY (embed via ldflags) and PRIVATE_KEY (store as the SQLFOX_MANIFEST_SIGNING_KEY secret)
//
// Sign the checksum manifest during release (produces <file>.sig, base64 Ed25519 signature):
//
//	go run scripts/sign-manifest.go -sign -key "$SQLFOX_MANIFEST_SIGNING_KEY" dist/archives/checksums.txt
//
// The running binary embeds the public key via:
//
//	-ldflags "-X sqlfox/internal/server.manifestSigningPublicKeyB64=<PUBLIC_KEY>"
//
// and then REQUIRES checksums.txt.sig to verify before trusting any checksum.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	genKey := flag.Bool("genkey", false, "generate a new Ed25519 signing keypair and print both keys")
	sign := flag.Bool("sign", false, "sign the given file, writing <file>.sig")
	keyB64 := flag.String("key", "", "base64-encoded Ed25519 private key (for -sign)")
	flag.Parse()

	switch {
	case *genKey:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			fail("key generation failed: %v", err)
		}
		fmt.Printf("PUBLIC_KEY=%s\n", base64.StdEncoding.EncodeToString(pub))
		fmt.Printf("PRIVATE_KEY=%s\n", base64.StdEncoding.EncodeToString(priv))
		fmt.Fprintln(os.Stderr, "Store PRIVATE_KEY offline (e.g. GitHub secret SQLFOX_MANIFEST_SIGNING_KEY). Embed PUBLIC_KEY via ldflags.")

	case *sign:
		if *keyB64 == "" {
			fail("-sign requires -key <base64 private key>")
		}
		args := flag.Args()
		if len(args) != 1 {
			fail("-sign requires exactly one file argument")
		}
		priv, err := base64.StdEncoding.DecodeString(strings.TrimSpace(*keyB64))
		if err != nil || len(priv) != ed25519.PrivateKeySize {
			fail("invalid private key: expected %d-byte base64 Ed25519 private key", ed25519.PrivateKeySize)
		}
		data, err := os.ReadFile(args[0])
		if err != nil {
			fail("cannot read %s: %v", args[0], err)
		}
		sig := ed25519.Sign(ed25519.PrivateKey(priv), data)
		outPath := args[0] + ".sig"
		if err := os.WriteFile(outPath, []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0644); err != nil {
			fail("cannot write %s: %v", outPath, err)
		}
		fmt.Printf("wrote %s\n", outPath)

	default:
		fail("usage: sign-manifest.go (-genkey | -sign -key <b64> <file>)")
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
