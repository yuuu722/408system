package repository

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNeo4jRejectsFailuresWithoutLeakingBodiesOrCredentials(t *testing.T) {
	for _, reply := range []struct {
		code int
		body string
	}{{401, "PRIVATE_ERROR_BODY"}, {200, `{"results":[],"errors":[{"code":"Neo.ClientError","message":"PRIVATE_ERROR_BODY"}]}`}, {200, `{"results":[],"errors":[]}`}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			require.True(t, ok)
			require.Equal(t, "neo4j", u)
			require.Equal(t, "PRIVATE_TEST_PASSWORD", p)
			w.WriteHeader(reply.code)
			w.Write([]byte(reply.body))
		}))
		client := &Neo4jGraph{URL: server.URL, Username: "neo4j", Password: "PRIVATE_TEST_PASSWORD"}
		_, e := client.Version(context.Background(), "scope")
		require.Error(t, e)
		require.NotContains(t, e.Error(), "PRIVATE_ERROR_BODY")
		require.NotContains(t, e.Error(), "PRIVATE_TEST_PASSWORD")
		server.Close()
	}
}
