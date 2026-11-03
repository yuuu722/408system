package providerhttp

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestLocalModelsNeverUseExternalProxyButPublicProvidersKeepIt(t *testing.T) {
	for _, host := range []string{"localhost:11434", "127.0.0.1:11434", "[::1]:11434", "host.docker.internal:11434", "ollama:11434", "neo4j:7474", "192.168.1.7:8090", "172.18.0.5:11434"} {
		r, _ := http.NewRequest("POST", "http://"+host+"/v1/chat/completions", nil)
		called := false
		u, e := proxy(r, func(*http.Request) (*url.URL, error) {
			called = true
			return url.Parse("http://external-proxy.invalid")
		})
		require.NoError(t, e)
		require.Nil(t, u)
		require.False(t, called)
		client := NewClient(time.Second)
		p, e := client.Transport.(*http.Transport).Proxy(r)
		require.NoError(t, e)
		require.Nil(t, p)
	}
	r, _ := http.NewRequest("POST", "https://api.provider.example/v1/chat/completions", nil)
	called := false
	p, e := proxy(r, func(*http.Request) (*url.URL, error) {
		called = true
		return url.Parse("http://external-proxy.invalid")
	})
	require.NoError(t, e)
	require.True(t, called)
	require.Equal(t, "external-proxy.invalid", p.Host)
}
