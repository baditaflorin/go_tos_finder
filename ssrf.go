package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

var safeDialer = &net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}

func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := (&net.Resolver{}).LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked IP %s for host %s", ip, host)
		}
	}
	for _, ip := range ips {
		conn, dErr := safeDialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dErr == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("dial failed for %s", host)
}

var safeTransport = &http.Transport{
	DialContext:           safeDialContext,
	TLSHandshakeTimeout:   5 * time.Second,
	ResponseHeaderTimeout: 6 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	MaxIdleConns:          32,
	IdleConnTimeout:       30 * time.Second,
	ForceAttemptHTTP2:     true,
	TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
}

func validateURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("missing host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked IP %s", ip)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := (&net.Resolver{}).LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked IP %s for host %s", ip, host)
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	cgnat := net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}
	if v4 := ip.To4(); v4 != nil && cgnat.Contains(v4) {
		return true
	}
	ula := net.IPNet{IP: net.IP{0xfc, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Mask: net.CIDRMask(7, 128)}
	if v6 := ip.To16(); v6 != nil && ip.To4() == nil && ula.Contains(v6) {
		return true
	}
	return false
}
