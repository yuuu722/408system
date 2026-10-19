"""Offline parser regression: synthetic files only, no downloads or AI requests."""
import io
import os
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch

from PIL import Image
from fastapi.testclient import TestClient

import server


class DocxParsingTests(unittest.TestCase):
    def docx(self, directory, body, media=True):
        path = Path(directory) / "synthetic.docx"
        with zipfile.ZipFile(path, "w") as archive:
            archive.writestr("word/document.xml", f'''<w:document
              xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
              xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
              xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
              <w:body>{body}</w:body></w:document>''')
            archive.writestr("word/_rels/document.xml.rels", '''<Relationships
              xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
              <Relationship Id="rImage" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>
              </Relationships>''')
            if media:
                buf = io.BytesIO()
                Image.new("RGB", (80, 60), "white").save(buf, "PNG")
                archive.writestr("word/media/image1.png", buf.getvalue())
        return path

    def test_failed_image_keeps_original(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.docx(directory, '<w:p><w:r><w:drawing><a:blip r:embed="rImage"/></w:drawing></w:r></w:p>')
            with patch.object(server, "structure", side_effect=ValueError("synthetic failure")):
                with self.assertLogs(level="ERROR"):
                    page = server.parse_docx(path)[0]
            self.assertTrue(page["Warnings"])
            self.assertTrue(any(b.get("Image", "").startswith("data:image/png;base64,") for b in page["Blocks"]))

    def test_image_text_keeps_document_order_and_unique_ids(self):
        body = '''<w:p><w:r><w:t>1. 内存问题</w:t></w:r></w:p>
          <w:p><w:r><w:drawing><a:blip r:embed="rImage"/></w:drawing></w:r></w:p>
          <w:p><w:r><w:t>A. 第一项 B. 第二项</w:t></w:r></w:p>'''
        with tempfile.TemporaryDirectory() as directory:
            path = self.docx(directory, body)
            result = [{"ID": "0", "Kind": "text", "Text": "图片中的必要条件"}]
            with patch.object(server, "structure", return_value=result):
                page = server.parse_docx(path)[0]
            self.assertLess(page["Text"].index("必要条件"), page["Text"].index("第一项"))
            ids = [b["ID"] for b in page["Blocks"]]
            self.assertEqual(len(ids), len(set(ids)))
            self.assertTrue(any(b.get("Image") for b in page["Blocks"]))

    def test_unreferenced_media_is_not_inserted(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.docx(directory, '<w:p><w:r><w:t>正文</w:t></w:r></w:p>')
            with patch.object(server, "structure") as recognize:
                page = server.parse_docx(path)[0]
            recognize.assert_not_called()
            self.assertEqual("正文", page["Text"])

    def test_table_ignores_xml_indentation(self):
        body = '''<w:tbl><w:tr><w:tc>
             <w:p><w:r><w:t>RAM</w:t></w:r></w:p>
             <w:p><w:r><w:t>内存</w:t></w:r></w:p>
          </w:tc><w:tc><w:p><w:r><w:t>8GB</w:t></w:r></w:p></w:tc></w:tr></w:tbl>'''
        with tempfile.TemporaryDirectory() as directory:
            page = server.parse_docx(self.docx(directory, body, media=False))[0]
            self.assertEqual("RAM 内存 | 8GB", page["Text"])

    def test_missing_image_warns_and_keeps_text(self):
        body = '<w:p><w:r><w:t>1. 内存问题</w:t><w:drawing><a:blip r:embed="rImage"/></w:drawing></w:r></w:p>'
        with tempfile.TemporaryDirectory() as directory:
            path = self.docx(directory, body, media=False)
            with patch.object(server, "structure") as recognize:
                page = server.parse_docx(path)[0]
            recognize.assert_not_called()
            self.assertTrue(page["Warnings"])
            self.assertEqual("1. 内存问题", page["Text"])

    def test_native_formula_still_warns(self):
        body = '''<w:p xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">
          <w:r><w:t>公式题</w:t></w:r><m:oMath><m:r><m:t>x</m:t></m:r></m:oMath></w:p>'''
        with tempfile.TemporaryDirectory() as directory:
            page = server.parse_docx(self.docx(directory, body, media=False))[0]
            self.assertTrue(any("公式" in w for w in page["Warnings"]))

    def test_logical_batch_http_and_auth(self):
        body = ''.join(f'<w:p><w:r><w:t>段落{i}</w:t></w:r></w:p>' for i in range(33))
        with tempfile.TemporaryDirectory() as directory:
            path = self.docx(directory, body, media=False)
            with patch.dict(os.environ, {"PARSER_TOKEN": "synthetic"}), TestClient(server.app) as client:
                files = {"file": ("synthetic.docx", path.read_bytes())}
                self.assertEqual(401, client.post('/parse', files=files).status_code)
                result = client.post('/parse', files=files, data={"start_page": 2, "page_limit": 1}, headers={"Authorization": "Bearer synthetic"})
                self.assertEqual(200, result.status_code)
                self.assertEqual(3, result.json()["TotalPages"])
                text = result.json()["Pages"][0]["Text"]
                self.assertTrue(text.startswith("段落16\n"))
                self.assertNotIn("段落32", text)

    def test_pdf_batch_recognizes_only_requested_page(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'synthetic.pdf'
            first, second = Image.new('RGB', (200, 200), 'white'), Image.new('RGB', (200, 200), 'gray')
            first.save(path, 'PDF', save_all=True, append_images=[second])
            first.close(); second.close()
            with patch.object(server, 'structure', return_value=[{"ID": "text", "Kind": "text", "Text": "合成扫描文字", "Box": [0, 0, 1, 1]}]) as recognize:
                pages = server.parse_pdf(path, start=2, limit=1)
            self.assertEqual(1, recognize.call_count)
            self.assertEqual([2], [p['Number'] for p in pages])
            self.assertEqual('normalized-top-left-displayed-page', pages[0]['CoordinateSpace'])


if __name__ == "__main__":
    unittest.main()
