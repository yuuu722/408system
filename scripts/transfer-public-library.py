#!/usr/bin/env python3
"""Transfer the public corpus between installations without user data or credentials."""
import argparse
from hashlib import sha256
import importlib.util
import json
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import uuid
import zipfile

spec = importlib.util.spec_from_file_location("environment_transfer", Path(__file__).with_name("transfer-environment.py"))
environment = importlib.util.module_from_spec(spec)
spec.loader.exec_module(environment)

OWNER = "40800000-0000-4000-8000-000000000001"
SCOPES = {
    "exam408_subjects": "true",
    "exam408_knowledge_points": "true",
    "exam408_knowledge_point_relations": "true",
    "point_frequency_sources": "true",
    "knowledge_bases": "t.is_public",
    "documents": "t.id IN (SELECT id FROM public_docs)",
    "document_revisions": "t.document_id IN (SELECT id FROM public_docs)",
    "document_revision_files": "t.document_id IN (SELECT id FROM public_docs)",
    "revision_pages": "t.revision_id IN (SELECT id FROM public_revisions)",
    "revision_chunks": "t.revision_id IN (SELECT id FROM public_revisions)",
    "document_parses": "t.document_id IN (SELECT id FROM public_docs)",
    "document_catalog": "t.document_id IN (SELECT id FROM public_docs)",
    "document_question_views": "t.document_id IN (SELECT id FROM public_docs)",
    "document_chunks": "t.id IN (SELECT id FROM public_chunks)",
    "learning_assets": "t.is_public AND (t.document_id IS NULL OR t.document_id IN (SELECT id FROM public_docs))",
    "knowledge_entities": """t.knowledge_base_id IN (SELECT id FROM public_bases)
        AND (t.evidence_chunk_id IS NULL OR t.evidence_chunk_id IN (SELECT id FROM public_chunks))""",
    "knowledge_relations": """t.knowledge_base_id IN (SELECT id FROM public_bases)
        AND t.from_entity_id IN (SELECT id FROM public_entities)
        AND t.to_entity_id IN (SELECT id FROM public_entities)
        AND (t.evidence_chunk_id IS NULL OR t.evidence_chunk_id IN (SELECT id FROM public_chunks))""",
    "knowledge_entity_evidence": """t.entity_id IN (SELECT id FROM public_entities)
        AND t.chunk_id IN (SELECT id FROM public_chunks)""",
    "exam408_entity_mappings": "t.entity_id IN (SELECT id FROM public_entities)",
    "exam408_resource_mappings": """coalesce(t.document_id IN (SELECT id FROM public_docs), true)
        AND coalesce(t.chunk_id IN (SELECT id FROM public_chunks), true)""",
    "question_family_aliases": """t.canonical_family_id IN (SELECT payload->>'FamilyID' FROM public_assets)
        AND t.legacy_family_id IN (SELECT payload->>'FamilyID' FROM public_assets)""",
}
VIEWS = {
    "public_bases": ("knowledge_bases", SCOPES["knowledge_bases"]),
    "public_docs": ("documents", "t.knowledge_base_id IN (SELECT id FROM public_bases)"),
    "public_revisions": ("document_revisions", SCOPES["document_revisions"]),
    "public_chunks": ("document_chunks", """t.document_id IN (SELECT id FROM public_docs)
        AND t.knowledge_base_id IN (SELECT id FROM public_bases)"""),
    "public_entities": ("knowledge_entities", SCOPES["knowledge_entities"]),
    "public_assets": ("learning_assets", SCOPES["learning_assets"]),
}
ROOT = Path(__file__).resolve().parents[1]
COMPOSE = ["docker", "compose", "--project-directory", str(ROOT)]
PSQL = ["psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "exam408", "-d", "exam408"]


def run(args, **options):
    result = subprocess.run(args, stderr=subprocess.PIPE, **options)
    if result.returncode:
        # Database errors can show a row containing private material.
        raise RuntimeError(f"命令失败：{' '.join(args[:3])}（退出码 {result.returncode}）")
    return result.stdout


def public_archive_path(value):
    prefix = "/data/uploads/"
    if not value.startswith(prefix) or "\\" in value or any(p in ("", ".", "..") for p in value[len(prefix):].split("/")):
        raise ValueError("公共资料包含无法移植的上传路径")
    return value[len(prefix):]


def export_rows(database_command, work):
    sql = "BEGIN ISOLATION LEVEL REPEATABLE READ;\n"
    for name, (table, scope) in VIEWS.items():
        sql += f"CREATE TEMP VIEW {name} AS SELECT * FROM {table} t WHERE {scope};\n"
    for table, scope in SCOPES.items():
        sql += f"SELECT '{table}';\nSELECT row_to_json(t) FROM {table} t WHERE {scope};\n"
    with (work / "rows").open("wb") as output:
        run([*database_command, *PSQL], input=sql.encode("utf-8"), stdout=output)


def read_rows(work):
    records = {table: [] for table in SCOPES}
    with (work / "rows").open(encoding="utf-8") as stream:
        for line in stream:
            if line.strip() in records:
                table = line.strip()
            else:
                row = json.loads(line)
                if "user_id" in row:
                    row["user_id"] = OWNER
                if table == "documents":
                    row["error_message"] = ""
                    if row["status"] in ("uploaded", "processing"):
                        row["status"] = "ready" if row["active_revision_id"] else "failed"
                        row["processing_stage"] = ""
                records[table].append(row)

    paths = {}
    for table in ("documents", "document_revision_files"):
        for row in records[table]:
            source = public_archive_path(row["storage_path"])
            name = sha256(source.encode("utf-8")).hexdigest()
            paths[source] = name
            row["storage_path"] = name
    public_documents = {row["id"] for row in records["documents"]}
    public_assets = {row["id"] for row in records["learning_assets"]}
    for row in records["learning_assets"]:
        payload = row["payload"]
        linked = [payload.get("DocumentID")]
        for key in ("Evidence", "VariantSources"):
            linked.extend(c.get("DocumentID") for c in payload.get(key) or [])
        if any(doc and doc not in public_documents for doc in linked):
            raise ValueError("公共资源引用了非公共或已删除的原文，请先检查其来源")
        if payload.get("ParentAssetID") and payload["ParentAssetID"] not in public_assets:
            payload["ParentAssetID"] = ""
    return records, paths


def build_package(records, paths, archive, output):
    if not records["knowledge_bases"] or not records["documents"]:
        raise ValueError("备份没有可导出的公共文档")
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=3) as package:
        seen = set()
        with tarfile.open(archive, "r|gz") as uploads:
            for entry in uploads:
                name = entry.name[2:] if entry.name.startswith("./") else entry.name
                if name not in paths:
                    continue
                if not entry.isfile() or name in seen:
                    raise ValueError("公共原文件不是唯一的普通文件")
                with uploads.extractfile(entry) as source, package.open("files/" + paths[name], "w", force_zip64=True) as target:
                    shutil.copyfileobj(source, target)
                seen.add(name)
        if seen != paths.keys():
            raise ValueError(f"备份缺少 {len(paths) - len(seen)} 个公共原文件")
        package.writestr("library.json", json.dumps({"format": 1, "tables": records}, ensure_ascii=False))
    print(f"导出 {len(records['documents'])} 份公共文档、{len(records['learning_assets'])} 条公共资源。")


def export(args):
    output = args.output.resolve()
    if not args.backup:
        if output.exists():
            raise ValueError("请选择尚不存在的输出文件")
        partial = output.with_name(output.name + "." + uuid.uuid4().hex + ".tmp")
        try:
            with tempfile.TemporaryDirectory() as folder:
                work = Path(folder)
                export_rows([*COMPOSE, "exec", "-T", "postgres"], work)
                records, paths = read_rows(work)
                archive = work / "uploads.tgz"
                with archive.open("wb") as stream:
                    run([*COMPOSE, "exec", "-T", "backend", "tar", "-czf", "-", "-C", "/data/uploads",
                         "--null", "--verbatim-files-from", "-T", "-"],
                        input=b"".join(path.encode("utf-8") + b"\0" for path in paths), stdout=stream)
                build_package(records, paths, archive, partial)
            partial.rename(output)
        finally:
            partial.unlink(missing_ok=True)
        print(f"共享资料包：{output}")
        return
    backup = args.backup.resolve()
    if output.exists() or backup == output.parent or backup in output.parents:
        raise ValueError("请选择原备份目录以外、尚不存在的输出文件")
    manifest = json.loads((backup / "manifest.json").read_text(encoding="utf-8"))
    environment.validate_manifest(manifest)
    for name in ("images.tar", "postgres-data.tgz", "uploads.tgz"):
        if environment.digest(backup / name) != manifest["sha256"][name]:
            raise ValueError(f"备份文件校验失败：{name}")
    info = environment.json_command(["docker", "info", "--format", "{{json .}}"])
    if info["OSType"] != "linux" or info["Architecture"] != manifest["architecture"]:
        raise ValueError("Docker 需要 Linux 容器模式和备份的相同 CPU 架构")
    name = "exam408-public-" + uuid.uuid4().hex
    image = manifest["images"]["postgres"]
    run(["docker", "image", "load", "-i", str(backup / "images.tar")], stdout=subprocess.DEVNULL)
    run(["docker", "volume", "create", name], stdout=subprocess.DEVNULL)
    partial = output.with_name(output.name + "." + name + ".tmp")
    try:
        environment.archive_volume(manifest["images"]["redis"], name, backup, "postgres-data", restore=True)
        run(["docker", "run", "-d", "--network", "none", "--name", name,
             "-v", f"{name}:/var/lib/postgresql/data", image], stdout=subprocess.DEVNULL)
        for _ in range(60):
            ready = subprocess.run(["docker", "exec", name, "pg_isready", "-U", "exam408", "-d", "exam408"],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("隔离数据库未就绪")
        with tempfile.TemporaryDirectory() as folder:
            work = Path(folder)
            export_rows(["docker", "exec", "-i", name], work)
            records, paths = read_rows(work)
            build_package(records, paths, backup / "uploads.tgz", partial)
        partial.rename(output)
        print(f"共享资料包：{output}")
    finally:
        partial.unlink(missing_ok=True)
        subprocess.run(["docker", "rm", "-f", name], capture_output=True)
        subprocess.run(["docker", "volume", "rm", name], capture_output=True)


def import_sql(records, path_prefix, output):
    output.write("BEGIN;\n")
    for table in SCOPES:
        rows = records[table]
        for row in rows:
            if "user_id" in row:
                row["user_id"] = OWNER
            if "storage_path" in row:
                row["storage_path"] = path_prefix + row["storage_path"]
            if table in ("knowledge_bases", "learning_assets"):
                row["is_public"] = True
        data = json.dumps(rows, ensure_ascii=False)
        marker = "$library_" + uuid.uuid4().hex + "$"
        while marker in data:
            marker = "$library_" + uuid.uuid4().hex + "$"
        output.write(f"CREATE TEMP TABLE src_{table} AS SELECT * FROM "
                     f"jsonb_populate_recordset(NULL::{table}, {marker}{data}{marker}::jsonb);\n")

    output.write("""
LOCK TABLE knowledge_bases, documents IN SHARE ROW EXCLUSIVE MODE;
CREATE TEMP TABLE subject_ids AS
SELECT s.id AS old, coalesce(d.id,s.id) AS new FROM src_exam408_subjects s
LEFT JOIN exam408_subjects d USING(code);
CREATE TEMP TABLE point_ids AS
SELECT s.id AS old, coalesce(d.id,s.id) AS new FROM src_exam408_knowledge_points s
LEFT JOIN exam408_knowledge_points d USING(code);
CREATE TEMP TABLE base_ids AS
SELECT s.id AS old, CASE WHEN s.is_system THEN d.id ELSE s.id END AS new
FROM src_knowledge_bases s CROSS JOIN (SELECT id FROM knowledge_bases WHERE is_system AND is_public) d;
DO $$ BEGIN
 IF (SELECT count(*) FROM base_ids) <> (SELECT count(*) FROM src_knowledge_bases) THEN
  RAISE EXCEPTION 'Initialize the destination system library first';
 END IF;
END $$;
UPDATE src_exam408_subjects t SET id=m.new FROM subject_ids m WHERE t.id=m.old;
INSERT INTO exam408_subjects SELECT * FROM src_exam408_subjects ON CONFLICT(code) DO NOTHING;
UPDATE src_exam408_knowledge_points t SET subject_id=m.new FROM subject_ids m WHERE t.subject_id=m.old;
UPDATE src_exam408_knowledge_points t SET parent_id=m.new FROM point_ids m WHERE t.parent_id=m.old;
UPDATE src_exam408_knowledge_points t SET id=m.new FROM point_ids m WHERE t.id=m.old;
INSERT INTO exam408_knowledge_points SELECT * FROM src_exam408_knowledge_points ON CONFLICT(code) DO NOTHING;
INSERT INTO knowledge_bases SELECT * FROM src_knowledge_bases WHERE NOT is_system;
CREATE TEMP TABLE active_revisions AS SELECT id,active_revision_id FROM src_documents;
UPDATE src_documents SET active_revision_id=NULL;
""")
    for table, columns in (
        ("exam408_knowledge_point_relations", ("from_point_id", "to_point_id")),
        ("point_frequency_sources", ("point_id",)),
        ("learning_assets", ("point_id",)),
        ("exam408_resource_mappings", ("knowledge_point_id",)),
        ("exam408_entity_mappings", ("knowledge_point_id",)),
    ):
        for column in columns:
            output.write(f"UPDATE src_{table} t SET {column}=m.new FROM point_ids m WHERE t.{column}=m.old;\n")
    for table in ("documents", "document_chunks", "knowledge_entities", "knowledge_relations"):
        output.write(f"UPDATE src_{table} t SET knowledge_base_id=m.new FROM base_ids m WHERE t.knowledge_base_id=m.old;\n")
    output.write("""
CREATE TEMP TABLE entity_ids AS
SELECT s.id AS old, coalesce(d.id,s.id) AS new
FROM src_knowledge_entities s LEFT JOIN knowledge_entities d USING(knowledge_base_id,name,entity_type);
UPDATE src_knowledge_entities t SET id=m.new FROM entity_ids m WHERE t.id=m.old;
UPDATE src_knowledge_relations t SET from_entity_id=m.new FROM entity_ids m WHERE t.from_entity_id=m.old;
UPDATE src_knowledge_relations t SET to_entity_id=m.new FROM entity_ids m WHERE t.to_entity_id=m.old;
UPDATE src_knowledge_entity_evidence t SET entity_id=m.new FROM entity_ids m WHERE t.entity_id=m.old;
UPDATE src_exam408_entity_mappings t SET entity_id=m.new FROM entity_ids m WHERE t.entity_id=m.old;
""")
    special = {"knowledge_bases", "exam408_subjects", "exam408_knowledge_points"}
    ignore = {"exam408_knowledge_point_relations", "point_frequency_sources", "question_family_aliases", "knowledge_entities", "knowledge_entity_evidence", "exam408_entity_mappings"}
    for table in SCOPES:
        if table in special:
            continue
        conflict = " ON CONFLICT DO NOTHING" if table in ignore else ""
        if table == "learning_assets":
            conflict = """ ON CONFLICT(seed_key) DO UPDATE SET
                user_id=excluded.user_id, point_id=excluded.point_id, title=excluded.title,
                payload=excluded.payload, status=excluded.status, revision=excluded.revision,
                document_id=excluded.document_id
                WHERE learning_assets.is_public"""
        selection = f"SELECT * FROM src_{table}"
        if table == "knowledge_relations":
            selection = """SELECT s.* FROM src_knowledge_relations s WHERE NOT EXISTS (
                SELECT 1 FROM knowledge_relations d WHERE d.knowledge_base_id=s.knowledge_base_id
                AND d.from_entity_id=s.from_entity_id AND d.to_entity_id=s.to_entity_id
                AND d.relation_type=s.relation_type AND d.source=s.source
                AND d.evidence_chunk_id IS NOT DISTINCT FROM s.evidence_chunk_id)"""
        output.write(f"INSERT INTO {table} {selection}{conflict};\n")
    output.write("""
UPDATE documents d SET active_revision_id=r.active_revision_id FROM active_revisions r WHERE d.id=r.id;
COMMIT;
""")


def read_package(package, directory):
    files = package.namelist()
    if len(files) != len(set(files)) or any(
        name != "library.json" and not re.fullmatch(r"files/[0-9a-f]{64}", name) for name in files
    ):
        raise ValueError("共享包含有无效或重复路径")
    contents = json.loads(package.read("library.json"))
    records = contents["tables"]
    if contents["format"] != 1 or set(records) != set(SCOPES):
        raise ValueError("共享包格式不匹配")
    if any(not isinstance(rows, list) or any(not isinstance(row, dict) for row in rows) for rows in records.values()):
        raise ValueError("共享包数据格式无效")
    paths = {row["storage_path"] for table in ("documents", "document_revision_files") for row in records[table]}
    if set(files) != {"library.json"} | {"files/" + path for path in paths}:
        raise ValueError("共享包中的原文件和数据库清单不一致")
    for name in files:
        if name == "library.json":
            continue
        with package.open(name) as source, (directory / PurePosixPath(name).name).open("wb") as target:
            shutil.copyfileobj(source, target)
    return records


def import_package(args):
    directory_name = "shared-" + uuid.uuid4().hex
    target = "/data/uploads/" + directory_name
    with tempfile.TemporaryDirectory() as folder:
        work = Path(folder)
        files = work / directory_name
        files.mkdir()
        with zipfile.ZipFile(args.package.resolve()) as package:
            records = read_package(package, files)
        sql = work / "import.sql"
        with sql.open("w", encoding="utf-8", newline="\n") as output:
            import_sql(records, target + "/", output)
        # Stream a tar of flat, validated names as the backend service user.
        archive = work / "uploads.tar"
        with tarfile.open(archive, "w") as payload:
            for path in files.iterdir():
                payload.add(path, arcname=path.name)
        run([*COMPOSE, "exec", "-T", "backend", "mkdir", "--", target], stdout=subprocess.DEVNULL)
        # Keep the new directory on an uncertain database outcome. Removing
        # committed originals would break citations if the connection dropped.
        with archive.open("rb") as payload:
            run([*COMPOSE, "exec", "-T", "backend", "tar", "-xf", "-", "-C", target],
                stdin=payload, stdout=subprocess.DEVNULL)
        with sql.open("rb") as payload:
            run([*COMPOSE, "exec", "-T", "postgres", *PSQL], stdin=payload)
    print(f"导入 {len(records['documents'])} 份公共文档。图谱服务将从数据库回放。")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="action", required=True)
    export_parser = actions.add_parser("export")
    export_parser.add_argument("--backup", type=Path)
    export_parser.add_argument("--output", type=Path, required=True)
    import_parser = actions.add_parser("import")
    import_parser.add_argument("--package", type=Path, required=True)
    args = parser.parse_args()
    try:
        (export if args.action == "export" else import_package)(args)
    except (OSError, ValueError, KeyError, RuntimeError, tarfile.TarError, zipfile.BadZipFile) as error:
        print(f"共享资料迁移失败：{error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
