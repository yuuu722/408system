import contextlib
import importlib.util
import io
import json
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from email.parser import BytesParser
from email import policy
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('folder_import', Path(__file__).parents[1] / 'import-public-folder.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class FakeAPI:
    failed = False
    calls = []
    def __init__(self, base):
        self.base = base
        self.docs = [{'ID': 'old', 'Status': 'ready'}]
        self.assets = [{'ID':'old-question','IsPublic':True,'Payload':{'DocumentID':'old'}},
                       {'ID':'another-library','IsPublic':True,'Payload':{'DocumentID':'elsewhere'}}]
    def request(self, method, route, data=None):
        self.calls.append((method,route))
        if route == '/auth/login': return {'token':'test','user':{'role':'admin'}}
        if route == '/knowledge-bases': return [{'ID':'public','Name':'408 系统公共知识库','IsPublic':True}]
        if route == '/system/capabilities': return {'max_upload_bytes':100000}
        if method == 'DELETE': return None
        if route == '/exam408/assets': return self.assets
        return json.loads(json.dumps(self.docs))
    def upload(self, kb, item):
        self.docs.append({'ID':'new','Status':'failed' if self.failed else 'ready','OriginalFilename':'new.txt','ErrorMessage':'synthetic parse failure'})
        return {'ID':'new'}


class ImportTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory()
        self.root=Path(self.temp.name)/'source';self.root.mkdir()
        self.report=Path(self.temp.name)/'report'
        self.file=self.root/'数据结构教材.txt';self.file.write_text('example',encoding='utf-8')
        self.args=SimpleNamespace(source=str(self.root),report_dir=str(self.report),plan=None,apply=True,
                                  replace=True,api='http://127.0.0.1:8080/api/v1',kb_id=None,wait_hours=1)
        FakeAPI.calls=[];FakeAPI.failed=False
    def tearDown(self): self.temp.cleanup()
    def run_import(self):
        with patch.object(m,'API',FakeAPI),patch('builtins.input',return_value='admin'),patch.object(m.getpass,'getpass',return_value='secret'),contextlib.redirect_stdout(io.StringIO()):
            m.run(self.args)
    def test_classification_uses_specific_file_name(self):
        self.assertEqual(m.classify(self.file,self.root),'textbook')
        for name,kind in [('408历年真题.pdf','past_exam'),('408真题解析.pdf','solution'),('数据结构课件.pdf','courseware'),('我的笔记.md','notes'),('未知文件.pdf','other')]:
            self.assertEqual(m.classify(self.root/name,self.root),kind)
    def test_success_only_deletes_original_documents_and_related_assets(self):
        self.run_import()
        deleted=[r for method,r in FakeAPI.calls if method=='DELETE']
        self.assertEqual(deleted,['/exam408/assets/old-question','/documents/old'])
        self.assertTrue(json.loads((self.report/'import-state.json').read_text())['Complete'])
    def test_streamed_upload_preserves_chinese_filename_and_type(self):
        received={}
        class Handler(BaseHTTPRequestHandler):
            def log_message(self,*args): pass
            def do_POST(self):
                raw=self.rfile.read(int(self.headers['Content-Length']))
                message=BytesParser(policy=policy.default).parsebytes(
                    ('Content-Type: '+self.headers['Content-Type']+'\r\nMIME-Version: 1.0\r\n\r\n').encode()+raw)
                for part in message.iter_parts():
                    name=part.get_param('name',header='Content-Disposition')
                    received[name]=part.get_payload(decode=True)
                    if name=='file': received['filename']=part.get_filename()
                self.send_response(202);self.end_headers();self.wfile.write(b'{"ID":"uploaded"}')
        server=HTTPServer(('127.0.0.1',0),Handler)
        thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        try:
            api=m.API('http://127.0.0.1:'+str(server.server_port)+'/api/v1');api.token='test'
            self.assertEqual(api.upload('public',m.inventory(self.root)[0])['ID'],'uploaded')
            self.assertEqual(received['file'],self.file.read_bytes())
            self.assertEqual(received['filename'],self.file.name)
            self.assertEqual(received['resource_type'],b'textbook')
        finally:
            server.shutdown();server.server_close();thread.join()
    def test_completed_batch_is_not_reimported(self):
        self.run_import()
        FakeAPI.calls=[]
        self.run_import()
        self.assertFalse(any(method=='DELETE' for method,_ in FakeAPI.calls))
    def test_failed_processing_keeps_all_old_resources(self):
        FakeAPI.failed=True
        with self.assertRaisesRegex(RuntimeError,'旧资源全部保留'): self.run_import()
        self.assertFalse(any(method=='DELETE' for method,_ in FakeAPI.calls))
    def test_archives_block_before_login_or_deletion(self):
        (self.root/'资料.zip').write_bytes(b'zip')
        with self.assertRaisesRegex(ValueError,'不支持'): self.run_import()
        self.assertEqual(FakeAPI.calls,[])
    def test_plan_only_has_no_network_or_changes(self):
        self.args.apply=False
        self.run_import()
        self.assertTrue((self.report/'classification.csv').exists())
        self.assertEqual(FakeAPI.calls,[])

if __name__=='__main__': unittest.main()
