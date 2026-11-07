#!/usr/bin/env python3
"""Reuse uploaded parses, pair separate answers, and audit local public questions.
Completeness is a structural check, never a proof of answer correctness.
"""
import argparse
import csv
import getpass
import io
import os
import importlib.util
import json
import re
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

spec = importlib.util.spec_from_file_location('answer_pair', Path(__file__).with_name('pair-answer-files.py'))
pairer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pairer)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class API:
    def __init__(self, base):
        target = urllib.parse.urlsplit(base)
        if (target.scheme not in {'http', 'https'} or target.hostname not in
                {'localhost', '127.0.0.1', '::1'} or target.username or target.password
                or target.query or target.fragment):
            raise ValueError('只允许本机接口地址，不允许内嵌密码、查询参数或重定向')
        self.base, self.token = base.rstrip('/'), ''
        self.opener = urllib.request.build_opener(NoRedirect())

    def request(self, method, route, data=None):
        headers = {'Content-Type': 'application/json'}
        if self.token:
            headers['Authorization'] = 'Bearer ' + self.token
        req = urllib.request.Request(self.base + route, method=method, headers=headers,
                                     data=None if data is None else json.dumps(data).encode())
        try:
            with self.opener.open(req, timeout=180) as response:
                raw = response.read()
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as exc:
            # Do not persist server response bodies, credentials or source text in reports.
            raise RuntimeError(f'{method} {route}: HTTP {exc.code}') from None
        except (urllib.error.URLError, TimeoutError, OSError):
            raise RuntimeError(f'{method} {route}: 连接失败或超时；检查本机服务') from None


def explanation_present(text):
    text = (text or '').strip()
    text = re.sub(r'^.*?(?:【解析】|解析[:：])', '', text)
    # Answer lists such as "1.A 2.B" are not worked explanations.
    return len(re.findall(r'[\u4e00-\u9fff]', text)) >= 8 or bool(
        re.search(r'\d.*[=＝].*\d', text)) or len(re.findall(r'[a-zA-Z]{2,}', text)) >= 8


def audit(assets, docs):
    rows = []
    for a in assets:
        p = a.get('Payload') or {}
        if a.get('Kind') != 'question' or not a.get('IsPublic') or p.get('DocumentID') not in docs:
            continue
        gaps = []
        stem = (p.get('Stem') or '').strip()
        options = [str(x).strip() for x in p.get('Options') or []]
        answer = p.get('Answer')
        if not stem:
            gaps.append('缺题干')
        if not 2 <= len(options) <= 6 or not all(options) or len(set(options)) != len(options):
            gaps.append('选项不完整或重复')
        if (p.get('AnswerOrigin') != 'source' or not isinstance(answer, int)
                or isinstance(answer, bool) or not 0 <= answer < len(options)):
            gaps.append('缺可靠资料答案')
        if not explanation_present(p.get('Explanation')):
            gaps.append('缺实质解析')
        if not a.get('PointID'):
            gaps.append('未匹配知识点')
        if not (p.get('Source') or '').strip():
            gaps.append('缺答案来源说明')
        if p.get('Issue'):
            gaps.append('存在解析异常')
        if re.search('如图|下图|图示', stem) and not p.get('Images'):
            gaps.append('需核对题图')
        rows.append({'AssetID': a['ID'], 'DocumentID': p['DocumentID'],
                     'Filename': docs[p['DocumentID']]['OriginalFilename'],
                     'Number': p.get('QuestionNumber', ''), 'Section': p.get('SectionKey', ''),
                     'Page': p.get('Page', 0), 'Complete': not gaps,
                     'Status': a.get('Status', ''), 'ReviewStatus': p.get('ReviewStatus', ''),
                     'Missing': '；'.join(gaps)})
    return rows


def question_document(d, catalogs):
    c = catalogs.get(d['ID'], {})
    name = d['OriginalFilename']
    kind = c.get('ResourceType', d.get('ResourceType', 'other'))
    if kind == 'solution':
        return False
    if kind in pairer.QUESTION_TYPES:
        return True
    if pairer.ANSWER_MARK.search(name):
        return False
    return (kind in pairer.QUESTION_TYPES or c.get('Purpose') in {'questions', 'both'}
            or bool(pairer.QUESTION_MARK.search(name)))


def mappings(path, docs):
    if not path:
        return []
    result, seen = [], set()
    with Path(path).open(encoding='utf-8-sig', newline='') as f:
        for row in csv.DictReader(f):
            q, a = row.get('QuestionDocumentID', '').strip(), row.get('AnswerDocumentID', '').strip()
            if not q and not a:
                continue
            qs, ans = row.get('QuestionSection', '').strip(), row.get('AnswerSection', '').strip()
            if q not in docs or a not in docs or q == a:
                raise ValueError('配对表包含不属于所选公共库的文件，或题目与答案为同一文件')
            key = (q, qs)
            if key in seen:
                raise ValueError('同一题目文件/章节存在多条映射，请消除歧义')
            seen.add(key)
            result.append((docs[q], docs[a], qs, ans))
    return result


# Reuse the alternate path on later checkpoints instead of making hundreds of files.
REPORT_PATHS = {}


def write_report_file(path, text, encoding):
    target = REPORT_PATHS.get(path, path)
    tmp = None
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(mode='w', encoding=encoding, newline='',
                                         dir=path.parent, prefix=path.name + '.',
                                         suffix='.tmp', delete=False) as f:
            tmp = Path(f.name)
            f.write(text)
        try:
            tmp.replace(target)
        except PermissionError:
            # Windows denies replacing a CSV open in Excel/WPS. Never truncate it.
            target = path.with_name(f'{path.stem}-latest-{uuid.uuid4().hex[:12]}{path.suffix}')
            tmp.replace(target)
            print(f'报告文件被占用或禁止替换，已另存：{target.resolve()}；原文件不是最新结果。', flush=True)
        REPORT_PATHS[path] = target
        return target
    except OSError as exc:
        raise RuntimeError(f'无法保存报告：{path.resolve()}（{type(exc).__name__}）；'
                           '已完成的数据处理保留。请关闭报告文件，或用 --report-dir 指定可写的新目录后重跑。') from None
    finally:
        if tmp is not None:
            try:
                tmp.unlink(missing_ok=True)
            except OSError:
                pass


def csv_report(fields, rows):
    buffer = io.StringIO(newline='')
    writer = csv.DictWriter(buffer, fieldnames=fields)
    writer.writeheader()
    for row in rows:
        writer.writerow({k: ("'" + str(v) if str(v).lstrip().startswith(('=', '+', '-', '@')) else v)
                         for k in fields for v in [row.get(k, '')]})
    return buffer.getvalue()


def save_report(folder, report):
    write_report_file(folder / 'report.json', json.dumps(report, ensure_ascii=False, indent=2), 'utf-8')
    fields = ['AssetID', 'DocumentID', 'Filename', 'Number', 'Section', 'Page',
              'Complete', 'Status', 'ReviewStatus', 'Missing']
    write_report_file(folder / 'questions.csv', csv_report(fields, report.get('Questions', [])), 'utf-8-sig')
    doc_fields = ['DocumentID', 'Filename', 'Status', 'Action', 'QuestionCount', 'Published', 'NeedsReview', 'Error']
    write_report_file(folder / 'documents.csv', csv_report(doc_fields, report.get('Documents', [])), 'utf-8-sig')
    template = folder / 'pairs-template.csv'
    if not template.exists() and template not in REPORT_PATHS:
        write_report_file(template, 'QuestionDocumentID,AnswerDocumentID,QuestionSection,AnswerSection\n', 'utf-8-sig')


def execute(api, kb, docs, catalogs, manual, apply, folder, auto_publish=False):
    by_id = {d['ID']: d for d in docs}
    pairs, unresolved = pairer.choose_pairs(docs, catalogs)
    explicit = {q['ID'] for q, _, _, _ in manual}
    plans = [(q, a, '', '') for q, a in pairs if q['ID'] not in explicit] + manual
    selected = {d['ID'] for d in docs if question_document(d, catalogs)} | {q['ID'] for q, _, _, _ in plans}
    if auto_publish:
        selected |= {d['ID'] for d in docs if catalogs.get(d['ID'], {}).get('ResourceType', d.get('ResourceType')) == 'textbook'}
    report = {'KnowledgeBaseID': kb, 'Apply': apply, 'Documents': [], 'Pairs': [],
              'ManualReviewFiles': unresolved, 'Questions': [], 'AuditCompleted': False,
              'Notice': '完整仅表示结构字段齐备；自动发布另须服务端重建题干选项与原文资料答案解析，不编造内容。', 'AutoPublish': auto_publish}
    save_report(folder, report)
    for d in docs:
        item = {'DocumentID': d['ID'], 'Filename': d['OriginalFilename'],
                'Status': d.get('Status'), 'Action': '保留知识库用途'}
        report['Documents'].append(item)
        if d['ID'] in selected:
            if d.get('Status') != 'ready':
                item['Action'] = '文档未就绪，跳过'
            elif not apply:
                item['Action'] = '计划复用解析抽题'
            else:
                try:
                    c = dict(catalogs.get(d['ID'], {}))
                    c.update(DocumentID=d['ID'], ResourceType=c.get('ResourceType', d.get('ResourceType', 'other')),
                             SubjectCode=c.get('SubjectCode', ''), Purpose=c.get('Purpose') or 'knowledge',
                             ReviewState=c.get('ReviewState') or 'pending', Reason='批量整理已有题目资料，复用解析，不自动发布')
                    if c['Purpose'] == 'knowledge':
                        c['Purpose'] = 'both'
                    api.request('PATCH', f"/documents/{d['ID']}/catalog", c)
                    item['Result'] = api.request('POST', f"/documents/{d['ID']}/organize", {})
                    item['Action'] = '已整理候选题'
                except RuntimeError as exc:
                    item['Error'] = str(exc)
        print(f"{item['Filename']}：{item['Action']}", flush=True)
        save_report(folder, report)
    for q, a, qs, ans in plans:
        item = {'QuestionDocumentID': q['ID'], 'AnswerDocumentID': a['ID'], 'Applied': 0}
        report['Pairs'].append(item)
        try:
            if any(x['DocumentID'] == q['ID'] and x.get('Error') for x in report['Documents']):
                raise RuntimeError('题目文件整理失败，本次不继续配对')
            body = {'AnswerDocumentID': a['ID'], 'QuestionSection': qs, 'AnswerSection': ans,
                    'OnlyIncomplete': True}
            for _ in range(100):
                preview = api.request('POST', f"/documents/{q['ID']}/answer-file", body)
                item['Skipped'] = preview.get('Skipped') or []
                if not preview.get('Matches'):
                    break
                item['PreviewCount'] = len(preview['Matches'])
                if not apply:
                    break
                result = api.request('POST', f"/documents/{q['ID']}/answer-file",
                                     dict(body, Apply=True, Fingerprint=preview['Fingerprint']))
                count = result.get('Applied', 0)
                if not count:
                    raise RuntimeError('配对未产生更新，停止此文件，保留现有结果')
                item['Applied'] += count
                save_report(folder, report)
            else:
                raise RuntimeError('达到配对批次上限，请核对报告')
        except RuntimeError as exc:
            item['Error'] = str(exc)
        save_report(folder, report)
    # The server reconstructs the question and revalidates answer evidence;
    # client field completeness alone is never enough to publish.
    if auto_publish and apply:
        for item in report['Documents']:
            if item['DocumentID'] not in selected or item.get('Status') != 'ready' or item.get('Error'):
                continue
            item['Published'] = 0
            try:
                item['ContentResolution'] = api.request('POST', f"/documents/{item['DocumentID']}/resolve-answers", {})
                save_report(folder, report)
                for _ in range(100):
                    result = api.request('POST', f"/documents/{item['DocumentID']}/publish-verified", {})
                    item['PublicationSkipped'] = result.get('Skipped') or []
                    count = result.get('Published', 0)
                    item['Published'] += count
                    save_report(folder, report)
                    if not count:
                        break
                else:
                    raise RuntimeError('达到自动发布批次上限，请检查报告')
                print(f"{item['Filename']}：自动校验发布 {item['Published']} 题", flush=True)
            except RuntimeError as exc:
                item['Error'] = str(exc)
            save_report(folder, report)
    try:
        rows = audit(api.request('GET', f'/knowledge-bases/{kb}/question-inventory') or [], by_id)
    except RuntimeError as exc:
        report['AuditError'] = str(exc)
        save_report(folder, report)
        raise
    report['Questions'] = rows
    report['AuditCompleted'] = True
    counts = {}
    for row in rows:
        counts[row['DocumentID']] = counts.get(row['DocumentID'], 0) + 1
    for item in report['Documents']:
        item['QuestionCount'] = counts.get(item['DocumentID'], 0)
        if item['DocumentID'] in selected and not item['QuestionCount']:
            item['NeedsReview'] = '未产生候选题：核对题型、用途或现有解析，不代表文件没有题目'
    report['Summary'] = {'TotalCandidates': len(rows), 'StructurallyComplete': sum(r['Complete'] for r in rows),
                         'Incomplete': sum(not r['Complete'] for r in rows),
                         'Available': sum(r['Status'] == 'ready' for r in rows),
                         'AutoPublished': sum(x.get('Published', 0) for x in report['Documents'])}
    save_report(folder, report)
    return report


def main():
    parser = argparse.ArgumentParser(description='本机已上传公共资料：复用解析抽题、配对答案、逐题检查；不重传或重跑OCR')
    parser.add_argument('--api', default='http://127.0.0.1:8080/api/v1')
    parser.add_argument('--kb-id')
    parser.add_argument('--pairs', help='人工确认的文件/章节配对CSV')
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--auto-publish', action='store_true', help='自动发布服务端原文校验通过且包含资料答案解析的完整题，无需逐题确认')
    parser.add_argument('--report-dir', default='.cache/question-batch')
    args = parser.parse_args()
    if args.auto_publish and not args.apply:
        parser.error('--auto-publish 必须同时指定 --apply')
    api = API(args.api)
    login = api.request('POST', '/auth/login', {'Login': input('管理员邮箱或用户名：').strip(),
                                              'Password': getpass.getpass('管理员密码（不显示）：')})
    if login['user']['role'] != 'admin':
        raise ValueError('必须使用管理员账号处理公共库')
    api.token = login['token']
    # New server contract is required before any catalog/organize mutations.
    caps = api.request('GET', '/system/capabilities') or {}
    if not caps.get('question_batch_enabled'):
        raise ValueError('后端尚不支持完整性保护，请先更新并重建启动 backend、worker')
    if args.auto_publish and not caps.get('question_auto_publish_enabled'):
        raise ValueError('后端尚不支持原文校验自动发布，请先重建 backend、worker')
    bases = api.request('GET', '/knowledge-bases') or []
    bases = [b for b in bases if b.get('IsPublic') and
             (b['ID'] == args.kb_id if args.kb_id else b['Name'] == '408 系统公共知识库')]
    if len(bases) != 1:
        raise ValueError('无法唯一确定公共库，请用 --kb-id 指定')
    kb = bases[0]['ID']
    docs = api.request('GET', f'/knowledge-bases/{kb}/documents') or []
    catalogs = {c['DocumentID']: c for c in api.request('GET', f'/knowledge-bases/{kb}/catalog') or []}
    manual = mappings(args.pairs, {d['ID']: d for d in docs})
    folder = Path(args.report_dir)
    report = execute(api, kb, docs, catalogs, manual, args.apply, folder, args.auto_publish)
    print(('已处理' if args.apply else '仅预览') + '；' + json.dumps(report['Summary'], ensure_ascii=False))
    for name in ['questions.csv', 'documents.csv', 'report.json']:
        path = folder / name
        print(f'{name} 最新结果：{REPORT_PATHS.get(path, path).resolve()}')
    print('原文校验通过且完整的题目已自动发布。其余保留为未修复记录，具体原因见报告。' if args.auto_publish
          else '缺失题目、答案、解析或歧义项见报告；可使用 --auto-publish 自动校验发布完整题目。')
    if any(x.get('Error') for x in report['Documents'] + report['Pairs']):
        raise SystemExit(1)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError, KeyboardInterrupt) as exc:
        raise SystemExit(f'批处理未完成：{exc or "已停止，检查报告后可重跑"}') from None
