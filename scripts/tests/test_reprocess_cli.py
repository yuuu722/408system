import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('reparse', Path(__file__).parents[1] / 'reprocess-public-documents.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class ReparseTests(unittest.TestCase):
    def test_resume_is_sequential_and_never_uploads_or_deletes(self):
        class API:
            calls = []
            def request(self, method, route, body=None):
                self.calls.append((method, route))
                if method == 'POST':
                    assert route.endswith('/reprocess')
                    return {}
                return [{'ID': 'a', 'Status': 'ready'}, {'ID': 'b', 'Status': 'ready'}]
        api = API()
        report = {'Completed': ['a']}
        saved = []
        m.run(api, 'public', [{'ID':'a','Status':'ready','OriginalFilename':'A.pdf'},
                             {'ID':'b','Status':'processing','OriginalFilename':'B.pdf'}], report,
              lambda v: saved.append(list(v['Completed'])), pause=lambda _:None)
        self.assertEqual(report['Completed'], ['a','b'])
        self.assertEqual(api.calls, [('GET','/knowledge-bases/public/documents')])
        self.assertEqual(saved,[['a','b']])

    def test_failure_stops_before_next_document(self):
        class API:
            calls = []
            def request(self, method, route, body=None):
                self.calls.append((method,route))
                if method=='POST':return {}
                return [{'ID':'a','Status':'failed','ErrorMessage':'OCR failed'}]
        api=API();report={}
        with self.assertRaisesRegex(RuntimeError,'OCR failed'):
            m.run(api,'public',[{'ID':'a','Status':'ready','OriginalFilename':'A.pdf'},
                               {'ID':'b','Status':'ready','OriginalFilename':'B.pdf'}],report,lambda _:None)
        self.assertEqual(api.calls[0],('POST','/documents/a/reprocess'))
        self.assertFalse(any('/documents/b/' in route for _,route in api.calls))
        self.assertEqual(report['Completed'],[])

if __name__=='__main__':unittest.main()
