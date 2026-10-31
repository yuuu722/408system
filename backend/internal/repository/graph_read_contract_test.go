package repository

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGraphReadsNeverTakeNamespaceWriteLock(t *testing.T) {
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Statements []struct{ Statement string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		q := body.Statements[0].Statement
		seen = append(seen, q)
		if strings.Contains(q, "RETURN nodes,edges") {
			w.Write([]byte(`{"results":[{"data":[{"row":[[],[]]}]}],"errors":[]}`))
		} else {
			w.Write([]byte(`{"results":[{"data":[]}],"errors":[]}`))
		}
	}))
	defer server.Close()
	n := Neo4jGraph{URL: server.URL}
	_, e := n.Read(context.Background(), "scope")
	require.NoError(t, e)
	_, e = n.Paths(context.Background(), "scope", []string{"内存"})
	require.NoError(t, e)
	for _, q := range seen {
		require.NotContains(t, q, "SET g.")
		require.NotContains(t, q, "readCounter")
	}
	require.Contains(t, seen[1], "LIMIT 8")
	require.Contains(t, seen[1], "LIMIT 16")
	require.NotContains(t, seen[1], "*0..2")
}
func TestGraphAuthenticationFailureIsSpecificAndDoesNotExposeSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); w.Write([]byte("SECRET_RESPONSE")) }))
	defer server.Close()
	n := Neo4jGraph{URL: server.URL, Password: "secret-credential"}
	_, e := n.Paths(context.Background(), "scope", []string{"内存"})
	require.Error(t, e)
	require.Contains(t, e.Error(), "认证失败")
	require.NotContains(t, e.Error(), "SECRET")
	require.NotContains(t, e.Error(), n.Password)
}
