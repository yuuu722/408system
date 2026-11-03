// Package providerhttp keeps internal model traffic off external HTTP proxies.
package providerhttp

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func internalHost(host string) bool {
	host = strings.ToLower(host)
	ip := net.ParseIP(host)
	return host == "localhost" || host == "host.docker.internal" || host == "gateway.docker.internal" || host == "ollama" || host == "neo4j" || ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}
func proxy(r *http.Request, fallback func(*http.Request) (*url.URL, error)) (*url.URL, error) {
	if internalHost(r.URL.Hostname()) {
		return nil, nil
	}
	return fallback(r)
}

var sharedTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(r *http.Request) (*url.URL, error) { return proxy(r, http.ProxyFromEnvironment) }
	return transport
}()

func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: sharedTransport}
}
