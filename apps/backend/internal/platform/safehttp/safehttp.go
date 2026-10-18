// Package safehttp builds HTTP clients for requests to URLs chosen by users
// (third-party status pages, webhooks). Unless private networks are allowed
// explicitly, connections to non-public addresses are refused at dial time,
// after DNS resolution, which also covers redirects and DNS rebinding.
// Proxy environment variables are ignored so the check applies to the
// destination itself.
package safehttp

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "64:ff9b::/96"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// PublicAddress reports whether ip may be contacted: no loopback, private,
// link-local, multicast, unspecified or special-purpose ranges.
func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// Options configures a client.
type Options struct {
	Timeout      time.Duration
	AllowPrivate bool
	// MaxRedirects; 0 refuses redirects (webhooks), otherwise only https
	// redirects are followed.
	MaxRedirects int
}

// NewClient returns a client applying the address policy.
func NewClient(o Options) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !o.AllowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !PublicAddress(ap.Addr()) {
				return errors.New("destination address is not public")
			}
			return nil
		}
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: o.Timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Timeout:   o.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= o.MaxRedirects {
				if o.MaxRedirects == 0 {
					return http.ErrUseLastResponse
				}
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return errors.New("redirect to non-https URL")
			}
			return nil
		},
	}
}
