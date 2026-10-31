package migrations

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 000001_initial.up.sql
var initial string

//go:embed 000002_operational_indexes.up.sql
var operationalIndexes string

//go:embed 000003_embedding_spaces.up.sql
var embeddingSpaces string

//go:embed 000004_document_progress.up.sql
var documentProgress string

//go:embed 000005_learning_foundation.up.sql
var learningFoundation string

//go:embed 000006_personal_resources.up.sql
var personalResources string

//go:embed 000007_document_revisions.up.sql
var documentRevisions string

//go:embed 000008_learning_plans.up.sql
var learningPlans string

//go:embed 000009_user_roles.up.sql
var userRoles string

//go:embed 000010_public_knowledge_bases.up.sql
var publicKnowledgeBases string

//go:embed 000011_graph_node_evidence.up.sql
var graphNodeEvidence string

//go:embed 000012_graph_relation_validation.up.sql
var graphRelationValidation string

//go:embed 000013_graph_entity_evidence.up.sql
var graphEntityEvidence string

//go:embed 000014_public_curriculum.up.sql
var publicCurriculum string

//go:embed 000015_management_lifecycle.up.sql
var managementLifecycle string

//go:embed 000016_uploaded_public_resources.up.sql
var uploadedPublicResources string

//go:embed 000017_document_catalog.up.sql
var documentCatalog string

//go:embed 000018_graph_sync.up.sql
var graphSync string

//go:embed 000019_graph_source_compat.up.sql
var graphSourceCompat string

//go:embed 000020_library_policy.up.sql
var libraryPolicy string

type migration struct {
	version int
	name    string
	sql     string
}

var all = []migration{
	{version: 1, name: "initial", sql: initial},
	{version: 2, name: "operational_indexes", sql: operationalIndexes},
	{version: 3, name: "embedding_spaces", sql: embeddingSpaces},
	{version: 4, name: "document_progress", sql: documentProgress},
	{version: 5, name: "learning_foundation", sql: learningFoundation},
	{version: 6, name: "personal_resources", sql: personalResources},
	{version: 7, name: "document_revisions", sql: documentRevisions},
	{version: 8, name: "learning_plans", sql: learningPlans},
	{version: 9, name: "user_roles", sql: userRoles},
	{version: 10, name: "public_knowledge_bases", sql: publicKnowledgeBases},
	{version: 11, name: "graph_node_evidence", sql: graphNodeEvidence},
	{version: 12, name: "graph_relation_validation", sql: graphRelationValidation},
	{version: 13, name: "graph_entity_evidence", sql: graphEntityEvidence},
	{version: 14, name: "public_curriculum", sql: publicCurriculum},
	{version: 15, name: "management_lifecycle", sql: managementLifecycle},
	{version: 16, name: "uploaded_public_resources", sql: uploadedPublicResources},
	{version: 17, name: "document_catalog", sql: documentCatalog},
	{version: 18, name: "graph_sync", sql: graphSync},
	{version: 19, name: "graph_source_compat", sql: graphSourceCompat},
	{version: 20, name: "library_policy", sql: libraryPolicy},
}

func Up(ctx context.Context, db *pgxpool.Pool) error {
	conn, err := db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(4082026)`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(4082026)`) }()

	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY,
		name text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Older phase-one installations predate schema_migrations. If the users
	// table is present, the initial migration has already been applied by the
	// application or the PostgreSQL Docker entrypoint, so record that baseline.
	var initialRecorded, usersExist bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=1)`).Scan(&initialRecorded); err != nil {
		return fmt.Errorf("read migration baseline: %w", err)
	}
	if !initialRecorded {
		if err = conn.QueryRow(ctx, `SELECT to_regclass('public.users') IS NOT NULL`).Scan(&usersExist); err != nil {
			return fmt.Errorf("inspect legacy schema: %w", err)
		}
		if usersExist {
			if _, err = conn.Exec(ctx, `INSERT INTO schema_migrations(version,name) VALUES(1,'initial') ON CONFLICT DO NOTHING`); err != nil {
				return fmt.Errorf("record legacy migration baseline: %w", err)
			}
		}
	}

	for _, m := range all {
		var applied bool
		if err = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, m.version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %06d: %w", m.version, err)
		}
		if applied {
			continue
		}
		if _, err = conn.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("apply migration %06d_%s: %w", m.version, m.name, err)
		}
		if _, err = conn.Exec(ctx, `INSERT INTO schema_migrations(version,name) VALUES($1,$2)`, m.version, m.name); err != nil {
			return fmt.Errorf("record migration %06d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}
