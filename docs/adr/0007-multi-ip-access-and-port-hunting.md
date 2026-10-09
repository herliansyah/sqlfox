# 0007: Multi-IP Network Access and Linear Port Hunting

> [!NOTE]
> **Amended by [ADR-0046](0046-secure-default-binding-and-ingress-hardening.md)**: The default network binding has been updated to strictly bind to `127.0.0.1` by default for zero network exposure, requiring the explicit `--lan` flag (or `-host 0.0.0.0`) to open port access across local networks.
> 
> **Amended by [ADR-0047](0047-single-instance-guard-and-lock-management.md)**: Sequential port hunting is reserved for collisions with other third-party services. When an active SqlFox instance is already running on the machine, subsequent launches hand off to the running instance and exit rather than spawning duplicate servers on new ports.

SqlFox binds to `127.0.0.1` by default (and `0.0.0.0` when `--lan` is specified) to enable tablet and remote LAN access, while protecting database credentials via an automated Remote Access Guard and linear port collision hunting.

## Considered Options

- **Strict `127.0.0.1` Binding**: Secure by default, but completely blocks developers from accessing the dashboard from tablets, phones, or LAN devices on the same network.
- **Unrestricted `0.0.0.0` Binding without Auth**: High security risk; anyone on the same Wi-Fi could access the database manager without authentication.
- **Selective Remote Access Guard + Port Hunting**: Binds to `0.0.0.0`, allows zero-friction instant access from `localhost`, requires Master Password verification for remote non-loopback IP requests, and scans sequential ports (8080 → 8130) if default ports are occupied.

## Consequences

Developers can effortlessly open SqlFox on mobile and tablet devices via local network IPs (`http://192.168.x.x:port`), while external network requests are strictly gated by the Remote Access Guard. Port collisions never crash the application on startup.
