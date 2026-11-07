import csv
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('question_batch', Path(__file__).parents[1] / 'complete-question-library.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

Q = {'ID': 'q', 'Status': 'ready', 'OriginalFilename': '2024年408真题题目.pdf', 'ResourceType': 'past_exam'}
A = {'ID': 'a', 'Status': 'ready', 'OriginalFilename': '2024年408真题答案.pdf', 'ResourceType': 'solution'}

class BatchTests(unittest.TestCase):
    def test_audit_distinguishes_answer_from_explanation_and_private(self):
        payload = {'DocumentID': 'q', 'Stem': '请选择正确叙述', 'Options': ['A内容', 'B内容'],
                   'Answer': 0, 'AnswerOrigin': 'source', 'Explanation': '1.A', 'Source': '原书第2页'}
        asset = {'ID': 'x', 'Kind': 'question', 'IsPublic': True, 'PointID': 'point', 'Payload': payload}
        rows = m.audit([asset, dict(asset, ID='private', IsPublic=False)], {'q': Q})
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]['Missing'], '缺实质解析')
        payload['Explanation'] = '因为线程共享进程的地址空间，所以开销较小。'
        self.assertTrue(m.audit([asset], {'q': Q})[0]['Complete'])
        self.assertNotIn('Stem', rows[0])

    def test_invalid_options_figures_and_boolean_answer(self):
        a = {'ID': 'x', 'Kind': 'question', 'IsPublic': True, 'Payload': {
            'DocumentID': 'q', 'Stem': '如下图所示', 'Options': ['A', 'A'],
            'Answer': True, 'AnswerOrigin': 'source'}}
        gaps = m.audit([a], {'q': Q})[0]['Missing']
        for text in ['选项不完整或重复', '缺可靠资料答案', '需核对题图']:
            self.assertIn(text, gaps)

    def test_apply_uses_cached_parse_and_fresh_preview(self):
        class API:
            def __init__(self): self.calls=[]; self.previews=0
            def request(self, method, route, body=None):
                self.calls.append((method, route, body))
                if route.endswith('/catalog'):
                    assert body['Purpose']=='both'
                    return {}
                if route.endswith('/organize'): return {'candidates': 1}
                if route.endswith('/question-inventory'): return []
                assert body['OnlyIncomplete'] is True
                if body.get('Apply'):
                    assert body['Fingerprint']=='fresh'
                    return {'Applied': 1}
                self.previews += 1
                return {'Fingerprint':'fresh','Matches':[1] if self.previews==1 else []}
        with tempfile.TemporaryDirectory() as tmp:
            api=API()
            report=m.execute(api,'kb',[Q,A],{},[],True,Path(tmp))
            self.assertEqual(report['Pairs'][0]['Applied'],1)
            self.assertTrue(report['AuditCompleted'])
            self.assertIn('未产生候选题',report['Documents'][0]['NeedsReview'])
            self.assertFalse(any(method=='DELETE' or any(x in path for x in ['publish','reprocess','upload'])
                                 for method,path,_ in api.calls))
            self.assertTrue(all('/a/organize' not in p for _,p,_ in api.calls))

    def test_dry_run_never_changes_records(self):
        class API:
            def request(self, method, route, body=None):
                if method=='GET': return []
                assert route.endswith('/answer-file') and not body.get('Apply')
                return {'Matches':[], 'Fingerprint':'fresh'}
        with tempfile.TemporaryDirectory() as tmp:
            result=m.execute(API(),'kb',[Q,A],{},[],False,Path(tmp))
            self.assertFalse(result['Apply'])
            self.assertEqual(result['Documents'][0]['Action'],'计划复用解析抽题')

    def test_auto_publish_uses_server_source_verification_after_resolution(self):
        class API:
            def __init__(self): self.calls=[]; self.rounds=0
            def request(self,method,route,body=None):
                self.calls.append(route)
                if route.endswith('/catalog'): return {}
                if route.endswith('/organize'): return {'candidates': 2}
                if route.endswith('/resolve-answers'): return {'Resolved': 1}
                if route.endswith('/publish-verified'):
                    self.rounds+=1
                    return {'Published':1 if self.rounds==1 else 0,'Skipped':[]}
                if route.endswith('/question-inventory'): return []
                if route.endswith('/question-diagnostics'): return {'DocumentID': 'q', 'Samples': []}
                raise AssertionError(route)
        with tempfile.TemporaryDirectory() as tmp:
            api=API()
            report=m.execute(api,'kb',[Q],{},[],True,Path(tmp),auto_publish=True)
            self.assertEqual(report['Summary']['AutoPublished'],1)
            self.assertLess(api.calls.index('/documents/q/resolve-answers'),api.calls.index('/documents/q/publish-verified'))
            self.assertFalse(any('assets/' in x for x in api.calls), 'never force client-supplied ready flags')

    def test_auto_detection_does_not_skip_other_books_or_combined_solutions(self):
        docs = [dict(Q, ID='other', OriginalFilename='2026操作系统.pdf', ResourceType='other'),
                dict(A, ID='combined', OriginalFilename='2009-2023 真题与解析打印版408.pdf'),
                dict(Q, ID='outline', OriginalFilename='2024年408考研大纲.pdf', ResourceType='other')]
        class API:
            def __init__(self): self.organized=[]
            def request(self,method,route,body=None):
                if route.endswith('/organize'):
                    assert body['AutoDetect'] is True
                    self.organized.append(route)
                    return {'candidates':1}
                if route.endswith('/catalog'): raise AssertionError('must not blindly change purpose before detection')
                if route.endswith('/resolve-answers'): return {'Resolved':0}
                if route.endswith('/publish-verified'): return {'Published':0}
                if route.endswith('/question-inventory'): return []
                if route.endswith('/question-diagnostics'): return {'Samples':[]}
                raise AssertionError(route)
        with tempfile.TemporaryDirectory() as tmp:
            api=API()
            result=m.execute(api,'kb',docs,{},[],True,Path(tmp),auto_publish=True)
            self.assertEqual(set(api.organized),{'/documents/other/organize','/documents/combined/organize'})
            self.assertIn('考试大纲',result['Documents'][2]['Action'])
            self.assertTrue((Path(tmp)/'diagnostics.json').exists())

    def test_diagnostics_only_is_read_only_and_excludes_syllabus(self):
        class API:
            def request(self,method,route,body=None):
                assert method=='GET' and route.endswith('/question-diagnostics')
                assert 'outline' not in route
                return {'Samples':[{'Text':'保存的原文样本'}]}
        with tempfile.TemporaryDirectory() as tmp:
            folder=Path(tmp)
            (folder/'report.json').write_text('previous report')
            path,errors=m.collect_diagnostics(API(),'kb',[Q,dict(Q,ID='outline',OriginalFilename='2024年408大纲.pdf')],folder)
            self.assertEqual(errors,0)
            self.assertEqual((folder/'report.json').read_text(),'previous report')
            self.assertEqual(len(json.loads(path.read_text())['Documents']),1)

    def test_manual_mapping_rejects_foreign_and_duplicate_ids(self):
        with tempfile.TemporaryDirectory() as tmp:
            p=Path(tmp)/'pairs.csv'
            p.write_text('QuestionDocumentID,AnswerDocumentID\nq,foreign\n',encoding='utf-8')
            with self.assertRaises(ValueError): m.mappings(p,{'q':Q,'a':A})
            p.write_text('QuestionDocumentID,AnswerDocumentID\nq,a\nq,a\n',encoding='utf-8')
            with self.assertRaises(ValueError): m.mappings(p,{'q':Q,'a':A})

    def test_local_api_rejects_credentials_and_external_hosts(self):
        for url in ['http://example.com','http://user:password@localhost:8080','http://localhost:8080?token=x']:
            with self.assertRaises(ValueError): m.API(url)
        self.assertIsNone(m.NoRedirect().redirect_request(None,None,302,'',{},'https://external.invalid'))

    def test_report_escapes_spreadsheet_cells_and_no_source_payload(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder=Path(tmp)
            m.save_report(folder, {'Questions':[{'AssetID':'x','DocumentID':'q','Filename':'=HYPERLINK(x)',
                'Number':'1','Section':'','Page':1,'Complete':False,'Status':'unavailable',
                'ReviewStatus':'pending','Missing':'缺实质解析'}]})
            with (folder/'questions.csv').open(encoding='utf-8-sig') as f:
                self.assertTrue(next(csv.DictReader(f))['Filename'].startswith("'="))
            self.assertFalse((folder/'report.json.tmp').exists())
            self.assertIn('Questions',json.loads((folder/'report.json').read_text()))

    def test_locked_reports_are_saved_elsewhere_and_reused(self):
        original_replace = Path.replace
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            for name in ['questions.csv', 'documents.csv', 'report.json']:
                (folder / name).write_text('locked original', encoding='utf-8')
            def locked_replace(source, target):
                if Path(target).name in {'questions.csv', 'documents.csv', 'report.json'}:
                    raise PermissionError('simulated Windows file lock')
                return original_replace(source, target)
            with patch.object(Path, 'replace', locked_replace):
                m.save_report(folder, {'Questions': [], 'Documents': [], 'checkpoint': 1})
                first = dict(m.REPORT_PATHS)
                m.save_report(folder, {'Questions': [], 'Documents': [], 'checkpoint': 2})
            for name in ['questions.csv', 'documents.csv', 'report.json']:
                self.assertEqual((folder/name).read_text(), 'locked original')
                self.assertEqual(m.REPORT_PATHS[folder/name], first[folder/name])
                self.assertTrue(first[folder/name].exists())
            self.assertEqual(json.loads(first[folder/'report.json'].read_text())['checkpoint'], 2)
            self.assertFalse(list(folder.glob('*.tmp')))

    def test_unwritable_report_directory_has_actionable_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            with patch.object(m.tempfile, 'NamedTemporaryFile', side_effect=PermissionError('denied')):
                with self.assertRaisesRegex(RuntimeError, '--report-dir'):
                    m.save_report(Path(tmp), {})

if __name__ == '__main__': unittest.main()
