#!/usr/bin/env python3
"""Pair separate answer files on the user's running local installation."""
import argparse
import getpass
import importlib.util
import json
import re
import unicodedata
import urllib.parse
from pathlib import Path

spec = importlib.util.spec_from_file_location('public_import', Path(__file__).with_name('import-public-folder.py'))
importer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(importer)

ANSWER_MARK = re.compile(r'答案|解析|解答|solution|answer', re.I)
QUESTION_TYPES = {'exercise', 'past_exam', 'mock_exam'}
QUESTION_MARK = re.compile(r'试卷|真题|习题|做题本|选择题|题集')
GENERIC = {'数据结构', '操作系统', '计算机操作系统', '计算机网络', '计算机组成原理', '408', '408考研'}


def identity(name):
    text = unicodedata.normalize('NFKC', Path(name).stem).lower()
    text = re.sub(r'^\s*\d{1,2}[.、_-]\s*', '', text)
    for term in ['参考答案与解析', '答案与解析', '参考答案', '答案解析', '习题解答', '习题答案',
                 '课后习题', '题目', '答案', '解析', '解答', '练习题', '习题', '题集', '题册', '做题本', 'solutions', 'answers']:
        text = text.replace(term, '')
    return re.sub(r'[^\w\u4e00-\u9fff]', '', text)


def choose_pairs(docs, catalogs):
    questions, answers = {}, {}
    skipped = []
    for d in docs:
        if d.get('Status') != 'ready':
            continue
        c = catalogs.get(d['ID'], {})
        kind = c.get('ResourceType', d.get('ResourceType', 'other'))
        name = d['OriginalFilename']
        key = identity(name)
        # A subject name alone does not identify a book or edition.
        if len(key) < 8 or key in GENERIC:
            skipped.append(name)
            continue
        is_answer = kind == 'solution' or bool(ANSWER_MARK.search(name))
        if is_answer:
            answers.setdefault(key, []).append(d)
        elif kind in QUESTION_TYPES or c.get('Purpose') in {'questions', 'both'} or QUESTION_MARK.search(name):
            questions.setdefault(key, []).append(d)
    pairs = []
    for key, qs in questions.items():
        candidates = answers.get(key, [])
        if len(qs) == 1 and len(candidates) == 1:
            pairs.append((qs[0], candidates[0]))
        else:
            skipped.extend(d['OriginalFilename'] for d in qs)
    used = {d['ID'] for pair in pairs for d in pair}
    skipped.extend(d['OriginalFilename'] for group in list(questions.values()) + list(answers.values()) for d in group if d['ID'] not in used)
    return pairs, sorted(set(skipped))


def execute(api, kb, docs, catalogs, pairs, apply):
    report = {'KnowledgeBaseID': kb, 'Applied': 0, 'Pairs': [], 'Errors': []}
    for q, a in pairs:
        item = {'Question': q['OriginalFilename'], 'Answer': a['OriginalFilename'], 'Applied': 0}
        report['Pairs'].append(item)
        try:
            if apply:
                c = dict(catalogs.get(q['ID'], {}))
                c.update(DocumentID=q['ID'], ResourceType=c.get('ResourceType', q.get('ResourceType', 'other')),
                         SubjectCode=c.get('SubjectCode', ''), Purpose=c.get('Purpose', 'knowledge'),
                         ReviewState=c.get('ReviewState', 'pending'), Reason='批量配对：复用解析提取候选，不自动发布')
                if c['Purpose'] == 'knowledge':
                    c['Purpose'] = 'both'
                api.request('PATCH', f"/documents/{q['ID']}/catalog", c)
                api.request('POST', f"/documents/{q['ID']}/organize", {})
            # A fresh preview is required for every 200-item transaction.
            for _ in range(100):
                body = {'AnswerDocumentID': a['ID']}
                preview = api.request('POST', f"/documents/{q['ID']}/answer-file", body)
                matches = preview.get('Matches') or []
                if not apply:
                    item['Preview'] = preview
                    break
                if not matches:
                    break
                result = api.request('POST', f"/documents/{q['ID']}/answer-file",
                                     dict(body, Apply=True, Fingerprint=preview['Fingerprint']))
                count = result.get('Applied', 0)
                if not count:
                    raise RuntimeError('未产生更新，已停止此文件配对，避免重复循环')
                item['Applied'] += count
                report['Applied'] += count
            else:
                raise RuntimeError('达到批次上限，请核对记录后继续')
            print(f"{q['OriginalFilename']} → {a['OriginalFilename']}：关联 {item['Applied']} 题（待审核）")
        except RuntimeError as e:
            item['Error'] = str(e)
            report['Errors'].append({'Question': q['OriginalFilename'], 'Error': str(e)})
    return report


def main():
    parser = argparse.ArgumentParser(description='本机公共库：预览或批量关联分文件答案，不删除文件、不自动发布')
    parser.add_argument('--api', default='http://127.0.0.1:8080/api/v1')
    parser.add_argument('--kb-id')
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--report', default='.cache/answer-pair/report.json')
    args = parser.parse_args()
    target = urllib.parse.urlsplit(args.api)
    if target.scheme not in {'http', 'https'} or target.hostname not in {'localhost', '127.0.0.1', '::1'}:
        parser.error('此工具仅连接本机服务；不要将管理员凭据发送给陌生地址')
    api = importer.API(args.api)
    result = api.request('POST', '/auth/login', {'Login': input('管理员邮箱或用户名：').strip(),
                                              'Password': getpass.getpass('管理员密码（不显示）：')})
    if result['user']['role'] != 'admin':
        raise ValueError('必须使用管理员账号')
    api.token = result['token']
    bases = api.request('GET', '/knowledge-bases') or []
    selected = [b for b in bases if b.get('IsPublic') and
                (b['ID'] == args.kb_id if args.kb_id else b['Name'] == '408 系统公共知识库')]
    if len(selected) != 1:
        raise ValueError('无法唯一确定公共库，请使用 --kb-id 指定')
    kb = selected[0]['ID']
    docs = api.request('GET', f'/knowledge-bases/{kb}/documents') or []
    catalogs = {c['DocumentID']: c for c in api.request('GET', f'/knowledge-bases/{kb}/catalog') or []}
    pairs, skipped = choose_pairs(docs, catalogs)
    print(f'找到 {len(pairs)} 对唯一同名资料；不能唯一识别的文件不会猜配。')
    report = execute(api, kb, docs, catalogs, pairs, args.apply)
    report['ManualReviewFiles'] = sorted(set(skipped))
    path = Path(args.report)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(f"{'已关联' if args.apply else '仅预览'} {report['Applied']} 题；报告：{path.resolve()}")
    print('已关联题目仍待审核。未唯一识别的资料请在网页指定文件与章节；本工具不删除文件或重跑OCR。')
    if report['Errors']:
        raise SystemExit('部分文件未完成，请查看报告中的 Errors；已完成关联保留。')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError) as exc:
        raise SystemExit(f'配对未完成：{exc}') from None
