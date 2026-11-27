// Package netguard blocks HTTP clients from dialing internal or special-use IP
// ranges when the destination URL comes from user-editable configuration.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

var (
	_, cgnatRange, _ = net.ParseCIDR("100.64.0.0/10")
	_, nat64Range, _ = net.ParseCIDR("64:ff9b::/96")
	awsIPv6IMDS      = net.ParseIP("fd00:ec2::254")
)

// IsBlockedDialIP reports whether an IP is unsafe to dial. allowLoopback and
// allowPrivate are for explicitly local endpoints only; remote callers must
// pass false for both.
func IsBlockedDialIP(ip net.IP, allowLoopback, allowPrivate bool) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
		if ip[0] == 0 {
			return true
		}
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if cgnatRange.Contains(ip) || nat64Range.Contains(ip) || ip.Equal(awsIPv6IMDS) {
		return true
	}
	if ip.IsLoopback() {
		return !allowLoopback
	}
	if !allowPrivate && ip.IsPrivate() {
		return true
	}
	return false
}

// IsExplicitLoopbackEndpoint reports whether rawURL names the local machine by
// literal. Public hostnames are never treated as loopback, even when DNS points
// at a local address.
func IsExplicitLoopbackEndpoint(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(u.Hostname())), ".")
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// SafeDialContext resolves the host first and rejects every blocked address
// before dialing, so DNS rebinding cannot reach loopback or internal services.
func SafeDialContext(allowLoopback, allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if IsBlockedDialIP(ip, allowLoopback, allowPrivate) {
				return nil, fmt.Errorf("blocked dial to restricted IP %s", ip.String())
			}
		}
		if len(ips) == 0 {
			return nil, errors.New("no IP addresses resolved")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
}
