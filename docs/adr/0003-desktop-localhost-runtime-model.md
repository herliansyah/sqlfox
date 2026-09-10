# 0003: Personal Localhost Desktop Topology for v1.0

To avoid enterprise multi-user auth overhead and maintain a lightweight single binary, SqlFox v1.0 operates strictly as a personal developer tool binding to `127.0.0.1` or packaged via desktop native webview.

## Considered Options

- **Shared Team Web Gateway**: Requires RBAC, session stores, user registration, and complex server-side certificate management.
- **Personal Localhost / Desktop App**: Zero authentication friction, direct local disk access for query dumps, and local encrypted credential storage via PBKDF2/Argon2 master password.

## Consequences

SqlFox remains lean, fast to launch (<1s), and memory bounded (<100MB), deferring multi-user collaborative complexities to future enterprise iterations.
