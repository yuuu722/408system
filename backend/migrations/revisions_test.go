package migrations

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestUpgradeKeepsLegacyActiveRevision(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, raw)
	require.NoError(t, e)
	defer admin.Close()
	name := "exam408_upgrade_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	_, e = db.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,name text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`)
	require.NoError(t, e)
	for _, m := range all {
		if m.version > 6 {
			break
		}
		_, e = db.Exec(ctx, m.sql)
		require.NoError(t, e)
		_, e = db.Exec(ctx, `INSERT INTO schema_migrations(version,name) VALUES($1,$2)`, m.version, m.name)
		require.NoError(t, e)
	}
	uid, kid, did := uuid.NewString(), uuid.NewString(), uuid.NewString()
	_, e = db.Exec(ctx, `INSERT INTO users(id,username,email,password_hash) VALUES($1,'legacy','legacy@example.invalid','synthetic')`, uid)
	require.NoError(t, e)
	_, e = db.Exec(ctx, `INSERT INTO knowledge_bases(id,user_id,name,description) VALUES($1,$2,'legacy','')`, kid, uid)
	require.NoError(t, e)
	_, e = db.Exec(ctx, `INSERT INTO documents(id,knowledge_base_id,user_id,filename,original_filename,mime_type,size,storage_path,resource_type,status) VALUES($1,$2,$3,'synthetic.txt','synthetic.txt','text/plain',1,'synthetic','notes','ready')`, did, kid, uid)
	require.NoError(t, e)
	_, e = db.Exec(ctx, `INSERT INTO document_chunks(id,document_id,knowledge_base_id,user_id,content,chunk_index,source_location,metadata,embedding) VALUES(gen_random_uuid(),$1,$2,$3,'legacy body',0,'legacy','{"embedding_space":"legacy-model"}','[1,0]')`, did, kid, uid)
	require.NoError(t, e)
	_, e = db.Exec(ctx, `INSERT INTO document_parses(document_id,payload) VALUES($1,'{"Pages":[{"Number":1,"Text":"legacy body"}]}')`, did)
	require.NoError(t, e)
	require.NoError(t, Up(ctx, db))
	require.NoError(t, Up(ctx, db))
	var rid, state string
	var count int
	require.NoError(t, db.QueryRow(ctx, `SELECT active_revision_id::text,status FROM documents WHERE id=$1`, did).Scan(&rid, &state))
	require.NotEmpty(t, rid)
	require.Equal(t, "ready", state)
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM revision_pages WHERE revision_id=$1`, rid).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(ctx, `SELECT count(*) FROM revision_chunks WHERE revision_id=$1 AND embedding IS NOT NULL`, rid).Scan(&count))
	require.Equal(t, 1, count)
	_, e = db.Exec(ctx, `DELETE FROM documents WHERE id=$1`, did)
	require.NoError(t, e)
}
