package migrations

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestRetireExamplesPreservesEditedAndUploadedResources(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, raw)
	require.NoError(t, e)
	defer admin.Close()
	name := "exam408_retire_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	uri, e := url.Parse(raw)
	require.NoError(t, e)
	uri.Path = "/" + name
	db, e := pgxpool.New(ctx, uri.String())
	require.NoError(t, e)
	defer func() {
		db.Close()
		_, e := admin.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, e)
	}()
	for _, m := range all {
		if m.version == 16 {
			break
		}
		_, e = db.Exec(ctx, m.sql)
		require.NoError(t, e)
	}
	var asset, owner, kid, edited string
	require.NoError(t, db.QueryRow(ctx, `SELECT a.id::text,a.user_id::text,kb.id::text FROM learning_assets a JOIN knowledge_bases kb ON kb.user_id=a.user_id AND kb.is_system WHERE a.kind='material' AND a.seed_key IS NOT NULL LIMIT 1`).Scan(&asset, &owner, &kid))
	require.NoError(t, db.QueryRow(ctx, `UPDATE learning_assets SET revision=2 WHERE id=(SELECT id FROM learning_assets WHERE seed_key IS NOT NULL AND id<>$1 LIMIT 1) RETURNING id::text`, asset).Scan(&edited))
	seeded, uploaded := uuid.NewString(), uuid.NewString()
	for _, d := range []struct{ id, name string }{{seeded, asset + "-r1.md"}, {uploaded, "my-408-book.pdf"}} {
		_, e = db.Exec(ctx, `INSERT INTO documents(id,user_id,knowledge_base_id,filename,original_filename,mime_type,size,storage_path,resource_type,status) VALUES($1,$2,$3,$4,$4,'text/plain',1,'synthetic','notes','ready')`, d.id, owner, kid, d.name)
		require.NoError(t, e)
	}
	_, e = db.Exec(ctx, `UPDATE learning_assets SET document_id=$2 WHERE id=$1`, asset, seeded)
	require.NoError(t, e)
	_, e = db.Exec(ctx, uploadedPublicResources)
	require.NoError(t, e)
	var count int
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM documents WHERE id=$1`, seeded).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM documents WHERE id=$1`, uploaded).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM learning_assets WHERE seed_key IS NOT NULL`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM learning_assets WHERE id=$1`, edited).Scan(&count))
	require.Equal(t, 1, count)
	// Startup is idempotent: rerunning cleanup never removes administrator uploads.
	_, e = db.Exec(ctx, uploadedPublicResources)
	require.NoError(t, e)
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM documents WHERE id=$1`, uploaded).Scan(&count))
	require.Equal(t, 1, count)
}
