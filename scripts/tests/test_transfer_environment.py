import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('transfer', Path(__file__).parents[1] / 'transfer-environment.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def manifest():
    files = {'code.bundle', 'runtime.env', 'images.tar'} | {f'{key}.tgz' for key in m.VOLUMES}
    return {'format':1,'volumes':dict(m.VOLUMES),'images':{s:f'exam408-transfer/{s}:test' for s in m.SERVICES},
            'sha256':{f:'a'*64 for f in files}}


class TransferTests(unittest.TestCase):
    def test_missing_volume_and_path_escape_rejected(self):
        value=manifest();m.validate_manifest(value)
        del value['volumes']['uploads']
        with self.assertRaises(ValueError):m.validate_manifest(value)
        for name in ('../outside', 'C:/private', 'a\\outside', '/absolute'):
            value=manifest();value['sha256'][name]='a'*64
            with self.assertRaises(ValueError):m.validate_manifest(value)

    def test_volume_archive_uses_readonly_source_and_no_network(self):
        calls=[]
        with patch.object(m,'command',side_effect=lambda args,**kw:calls.append(args)):
            m.archive_volume('helper','actual-uploads',Path('/tmp/package'),'uploads')
        args=calls[0]
        self.assertIn('none',args)
        self.assertIn('type=volume,source=actual-uploads,target=/volume,readonly',args)
        self.assertEqual(args[-1],'tar -czpf /backup/uploads.tgz -C /volume .')
        self.assertNotIn('pull',args)

    def test_damaged_archive_never_creates_project_or_volumes(self):
        with tempfile.TemporaryDirectory() as folder:
            folder=Path(folder);value=manifest();value['architecture']='x86_64'
            (folder/'manifest.json').write_text(json.dumps(value))
            args=type('Args',(),dict(bundle=str(folder),project_directory=str(folder/'new'),project_name='exam408-copy-test',skip_ollama=True))()
            def fake(args,**kwargs):
                if args[:3]==['docker','volume','ls']:return ''
                if args[:2]==['docker','ps']:return ''
                raise AssertionError('unexpected mutation: '+str(args))
            with patch.dict(m.os.environ,{},clear=True),patch.object(m,'json_command',return_value={'OSType':'linux','Architecture':'x86_64'}),patch.object(m,'command',side_effect=fake):
                with self.assertRaisesRegex(ValueError,'损坏或缺失'):m.restore_environment(args)
            self.assertFalse((folder/'new').exists())

    def test_existing_destination_is_never_overwritten(self):
        with tempfile.TemporaryDirectory() as folder:
            folder=Path(folder);value=manifest();value['architecture']='x86_64'
            (folder/'manifest.json').write_text(json.dumps(value))
            args=type('Args',(),dict(bundle=str(folder),project_directory=str(folder),project_name='exam408-copy-test',skip_ollama=True))()
            with patch.dict(m.os.environ,{},clear=True),patch.object(m,'json_command',return_value={'OSType':'linux','Architecture':'x86_64'}):
                with self.assertRaisesRegex(ValueError,'目录已存在'):m.restore_environment(args)

if __name__=='__main__':unittest.main()
