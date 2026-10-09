# 0046: Secure-by-Default Binding and Ingress Hardening

## Status
Accepted (Amends and supersedes default network binding in ADR-0007)

## Context
ADR-0007 introduced binding to `0.0.0.0` by default to streamline tablet and remote LAN access. However:
1. Operating with an open listener on `0.0.0.0` in untrusted Wi-Fi environments (cafés, offices, coworking spaces) exposes the database manager to local network scanning without explicit user consent.
2. The initial Master PIN format (`fox-%04d`) offered only 10,000 combinations, which was vulnerable to rapid brute-force enumeration on `POST /api/auth/unlock` without rate limiting or lockout protection.
3. The localhost trust model permitted unauthenticated API execution for loopback clients without validating `Host` or `Origin` headers, leaving users vulnerable to DNS Rebinding and browser-based Cross-Site Request Forgery (CSRF).
4. The self-update engine accepted arbitrary `asset_url` parameters without release origin filtering or cryptographic digest verification against official release manifests.

## Decisions

### 1. Secure-by-Default Ingress Binding
- SqlFox strictly binds to loopback (`127.0.0.1`) by default.
- Access across local area networks requires explicit operator activation via the `--lan` CLI flag (or `-host 0.0.0.0`).
- Console output suppresses the Master PIN and network URL unless `--lan` mode is explicitly active, eliminating shoulder-surfing exposure during local sessions.

### 2. High-Entropy Master PIN & Remote Lockout Policy
- Master PIN entropy is upgraded from 4 numeric digits to 6 random alphanumeric characters (`fox-[a-z0-9]{6}`, ~2.17 billion combinations).
- The `Remote Access Guard` enforces an in-memory `Remote Lockout Policy`: a remote IP address failing 5 consecutive authentication attempts on `/api/auth/unlock` is quarantined for 10 minutes (`429 Too Many Requests`).

### 3. DNS Rebinding & Cross-Origin CSRF Guard
- Ingress middleware inspects the HTTP `Host` header, accepting only approved loopback hostnames (`localhost`, `127.0.0.1`, `[::1]`) and the explicitly bound LAN interface when `--lan` is specified. Unrecognized host headers are rejected with `403 Forbidden`.
- State-mutating API endpoints (`POST`, `PUT`, `DELETE`, `PATCH`) verify `Origin` / `Sec-Fetch-Site` to prevent unauthorized cross-origin execution from malicious websites.
- The `GET /api/auth/master-key` endpoint is guarded to return PIN data only when `--lan` mode is enabled; in standard localhost mode, it indicates `{ "lan_enabled": false }`.

### 4. Fail-Closed Release Manifest Verification
- The self-update handler accepts only targeted version requests, disallowing arbitrary external asset URLs.
- Binary downloads are restricted strictly to official GitHub release URLs under `github.com/herliansyah/sqlfox/releases/download/...`.
- The engine downloads and parses the official release manifest (`checksums.txt` / `SHA256SUMS`), verifies the downloaded archive's SHA-256 digest, and fails closed if digests mismatch or if the manifest is unavailable.

## Consequences
- Guaranteed zero network exposure on untrusted public Wi-Fi networks by default.
- Complete mitigation of local DNS Rebinding and browser CSRF vectors.
- Total defense against automated Master PIN brute-forcing across remote LAN clients.
- Tamper-proof, cryptographically verified binary self-updates preserving supply-chain integrity.
