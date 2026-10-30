package config

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestKAGConfigurationRequiresExplicitGraphEndpoint(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("JWT_SECRET", strings.Repeat("s", 40))
	t.Setenv("ASSISTANT_MODE", "kag")
	t.Setenv("GRAPH_BACKEND", "postgres")
	_, e := Load()
	require.Error(t, e)
	t.Setenv("GRAPH_BACKEND", "neo4j")
	t.Setenv("NEO4J_PASSWORD", "synthetic-password")
	t.Setenv("NEO4J_URL", "bolt://localhost:7687")
	_, e = Load()
	require.Error(t, e)
	t.Setenv("NEO4J_URL", "http://localhost:7474")
	c, e := Load()
	require.NoError(t, e)
	require.Equal(t, "kag", c.AssistantMode)
}
