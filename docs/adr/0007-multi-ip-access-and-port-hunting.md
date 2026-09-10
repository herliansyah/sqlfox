# 0007: Multi-IP Network Access and Linear Port Hunting

SqlFox binds to `0.0.0.0` by default to enable tablet and remote LAN access, while protecting database credentials via an automated Remote Access Guard and linear port collision hunting.

## Considered Options

- **Strict `127.0.0.1` Binding**: Secure by default, but completely blocks developers from accessing the dashboard from tablets, phones, or LAN devices on the same network.
- **Unrestricted `0.0.0.0` Binding without Auth**: High security risk; anyone on the same Wi-Fi could access the database manager without authentication.
- **Selective Remote Access Guard + Port Hunting**: Binds to `0.0.0.0`, allows zero-friction instant access from `localhost`, requires Master Password verification for remote non-loopback IP requests, and scans sequential ports (8080 → 8130) if default ports are occupied.

## Consequences

Developers can effortlessly open SqlFox on mobile and tablet devices via local network IPs (`http://192.168.x.x:port`), while external network requests are strictly gated by the Remote Access Guard. Port collisions never crash the application on startup.
