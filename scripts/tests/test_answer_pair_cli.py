import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('answer_pair', Path(__file__).parents[1] / 'pair-answer-files.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class PairTests(unittest.TestCase):
    def test_preserves_edition_and_does_not_pair_subject_alone(self):
        self.assertEqual(m.identity('2024年408真题题目.pdf'), m.identity('2024年408真题答案.pdf'))
        self.assertNotEqual(m.identity('2024年408真题.pdf'), m.identity('2025年408真题答案.pdf'))
        docs = [{'ID':'q','Status':'ready','OriginalFilename':'操作系统习题.pdf','ResourceType':'exercise'},
                {'ID':'a','Status':'ready','OriginalFilename':'操作系统答案.pdf','ResourceType':'solution'}]
        self.assertEqual(m.choose_pairs(docs, {})[0], [])
    def test_duplicate_answer_files_require_review(self):
        docs = [{'ID':'q','Status':'ready','OriginalFilename':'2024年408真题题目.pdf','ResourceType':'exercise'},
                {'ID':'a','Status':'ready','OriginalFilename':'2024年408真题答案.pdf','ResourceType':'solution'}]
        self.assertEqual(len(m.choose_pairs(docs, {})[0]), 1)
        self.assertEqual(m.choose_pairs(docs+[dict(docs[1], ID='copy')], {})[0], [])
    def test_dry_run_never_writes(self):
        class API:
            def request(self, method, route, body):
                assert method == 'POST' and route.endswith('/answer-file')
                assert not body.get('Apply')
                return {'Matches':[], 'Fingerprint':'v1'}
        report=m.execute(API(),'kb',[],{},[({'ID':'q','OriginalFilename':'Q'}, {'ID':'a','OriginalFilename':'A'})],False)
        self.assertEqual(report['Applied'],0)

    def test_apply_organizes_and_requires_fresh_preview_without_publishing(self):
        class API:
            previews=0
            calls=[]
            def request(self, method, route, body):
                self.calls.append((method, route, body))
                if route.endswith('/catalog'):
                    assert body['Purpose']=='both'
                    return body
                if route.endswith('/organize'):return {}
                if body.get('Apply'):
                    assert body['Fingerprint']=='fresh'
                    return {'Applied':2}
                self.previews+=1
                return {'Matches':[1,2] if self.previews==1 else [],'Fingerprint':'fresh'}
        api=API()
        result=m.execute(api,'kb',[],{},[({'ID':'q','OriginalFilename':'Q','ResourceType':'exercise'}, {'ID':'a','OriginalFilename':'A'})],True)
        self.assertEqual(result['Applied'],2)
        self.assertEqual(api.previews,2)
        self.assertFalse(any(method=='DELETE' or 'publish' in route for method,route,_ in api.calls))

if __name__ == '__main__': unittest.main()
