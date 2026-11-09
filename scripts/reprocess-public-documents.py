#!/usr/bin/env python3
"""Reparse existing local public documents sequentially; never upload/delete."""
import argparse
import getpass
import importlib.util
import json
import time
import urllib.parse
from pathlib import Path

spec = importlib.util.spec_from_file_location('public_import', Path(__file__).with_name('import-public-folder.py'))
importer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(importer)


def run(api, kb, docs, report, save, timeout=7200, pause=time.sleep, now=time.monotonic):
    completed = report.setdefault('Completed', [])
    for doc in docs:
        did = doc['ID']
        if did in completed:
            continue
        print('重新解析：' + doc['OriginalFilename'], flush=True)
        # Resume an in-flight task without enqueueing it a second time.
        if doc['Status'] != 'processing':
            api.request('POST', f'/documents/{did}/reprocess', {})
        deadline = now() + timeout
        while True:
            current = next((d for d in api.request('GET', f'/knowledge-bases/{kb}/documents') or [] if d['ID'] == did), None)
            if current is None:
                raise RuntimeError('资料已不在该公共库，停止处理')
            if current['Status'] == 'failed':
                raise RuntimeError(f"{doc['OriginalFilename']}：{current.get('ErrorMessage', '处理失败')}")
            if current['Status'] == 'ready':
                completed.append(did)
                save(report)
                print(f"完成 {len(completed)}：{doc['OriginalFilename']}", flush=True)
                break
            if now() >= deadline:
                raise RuntimeError('等待超时，任务仍在后台处理；稍后运行同一命令继续')
            print(f"  {current.get('ProcessingStage', 'processing')} · {current.get('Progress', 0)}%", flush=True)
            pause(10)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--api', default='http://127.0.0.1:8080/api/v1')
    parser.add_argument('--kb-id')
    parser.add_argument('--document-id', action='append', default=[])
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--report', default='.cache/reparse/2026.11.04.3.json')
    args = parser.parse_args()
    target = urllib.parse.urlsplit(args.api)
    if target.scheme not in {'http', 'https'} or target.hostname not in {'localhost', '127.0.0.1', '::1'}:
        parser.error('仅允许连接本机服务')
    api = importer.API(args.api)
    # Capabilities require authentication; do not inspect .env or store credentials.
    login = api.request('POST', '/auth/login', {'Login': input('管理员邮箱或用户名：').strip(),
                                              'Password': getpass.getpass('管理员密码（不显示）：')})
    if login['user']['role'] != 'admin':
        raise ValueError('必须使用公共库管理员账号')
    api.token = login['token']
    cap = api.request('GET', '/system/capabilities')
    compatible = {'2026.11.04.3', '2026.11.06.1', '2026.11.06.2', '2026.11.06.3', '2026.11.07.1', '2026.11.07.2', '2026.11.07.3', '2026.11.07.4', '2026.11.07.5', '2026.11.07.6', '2026.11.07.7', '2026.11.08.1', '2026.11.08.2', '2026.11.08.3', '2026.11.09.1', '2026.11.09.2', '2026.11.09.3'}
    if cap.get('release') not in compatible:
        raise ValueError('服务端不是本工具支持的解析版本，请先拉取源码并运行start-kag.ps1')
    bases = [b for b in api.request('GET', '/knowledge-bases') or [] if b.get('IsPublic') and
             (b['ID'] == args.kb_id if args.kb_id else b['Name'] == '408 系统公共知识库')]
    if len(bases) != 1:
        raise ValueError('无法唯一确定公共库，请使用--kb-id指定')
    kb = bases[0]['ID']
    docs = api.request('GET', f'/knowledge-bases/{kb}/documents') or []
    if args.document_id:
        docs = [d for d in docs if d['ID'] in args.document_id]
        if {d['ID'] for d in docs} != set(args.document_id):
            raise ValueError('指定资料不全属于该公共库')
    docs = [d for d in docs if Path(d['OriginalFilename']).suffix.lower() in {'.pdf', '.doc', '.docx'}]
    print(f'共{len(docs)}份现有资料；逐份重新解析，不删除、不重新上传。')
    for d in docs:
        print('  ' + d['OriginalFilename'])
    if not args.apply:
        print('仅预览，添加--apply执行。')
        return
    path = Path(args.report)
    report = json.loads(path.read_text(encoding='utf-8')) if path.exists() else {'KnowledgeBaseID': kb, 'Release': cap['release'], 'Completed': []}
    if report.get('KnowledgeBaseID') != kb or report.get('Release') not in compatible:
        raise ValueError('报告属于其他库或解析版本，请用--report指定新报告')
    path.parent.mkdir(parents=True, exist_ok=True)
    def save(value):
        tmp = path.with_suffix('.tmp')
        tmp.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding='utf-8')
        tmp.replace(path)
    run(api, kb, docs, report, save)
    print('重新处理完成，请检查解析结果并审核题目；不自动发布题目。')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError) as exc:
        raise SystemExit(str(exc)) from None
