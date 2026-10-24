"""Bounded transport regression; no large fixture or model download required."""
import io
import os
import unittest
from unittest.mock import patch

from fastapi import HTTPException, UploadFile
import server


class UploadLimitTests(unittest.TestCase):
    def test_oversize_remains_413_and_releases_lock(self):
        with patch.dict(os.environ, {"PARSER_TOKEN": "synthetic"}), patch.object(server, "MAX_BYTES", 4):
            with self.assertRaises(HTTPException) as caught:
                server.parse(UploadFile(io.BytesIO(b"12345"), filename="test.pdf"), "Bearer synthetic", 1, 1)
            self.assertEqual(413, caught.exception.status_code)
            self.assertFalse(server.lock.locked())

    def test_input_is_copied_in_bounded_blocks(self):
        reads = []
        class Source(io.BytesIO):
            def read(self, size=-1):
                reads.append(size)
                if size < 0 or size > 1024 * 1024:
                    raise AssertionError("unbounded file read")
                return super().read(size)
        class Document:
            def __enter__(self): return self
            def __exit__(self, *args): pass
            def __len__(self): return 1
        data = b"synthetic" * 300000
        with patch.dict(os.environ, {"PARSER_TOKEN": "synthetic"}), patch.object(server, "MAX_BYTES", len(data)), \
             patch.object(server.pdfium, "PdfDocument", return_value=Document()), \
             patch.object(server, "parse_pdf", return_value=[{"Number": 1, "Text": "synthetic"}]):
            result = server.parse(UploadFile(Source(data), filename="test.pdf"), "Bearer synthetic", 1, 1)
        self.assertEqual(1, result["TotalPages"])
        self.assertGreater(len(reads), 2)
        self.assertFalse(server.lock.locked())


if __name__ == '__main__': unittest.main()
