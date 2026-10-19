"""Manual integration check against running services; creates an isolated test account.
No service startup, no user-document reprocessing. A successful chat calls the configured LLM.
"""
import io
import json
from pathlib import Path
import secrets
import time
import urllib.request
import urllib.error


def request(url, payload=None, token=None, content_type='application/json'):
    headers = {'Content-Type': content_type}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    data = json.dumps(payload).encode() if isinstance(payload, dict) else payload
    with urllib.request.urlopen(urllib.request.Request(url, data=data, headers=headers), timeout=110) as r:
        return json.load(r)


def pdf_bytes():
    # Tiny born-digital PDF fixture, deliberately not an OCR or layout-quality test.
    text = 'Virtual memory provides address isolation and uses disk to extend physical memory.'
    stream = f'BT /F1 12 Tf 40 750 Td ({text}) Tj ET'.encode()
    objects = [b'<< /Type /Catalog /Pages 2 0 R >>', b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
        b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',
        b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
        b'<< /Length ' + str(len(stream)).encode() + b' >>\nstream\n' + stream + b'\nendstream']
    f = io.BytesIO(); f.write(b'%PDF-1.4\n'); offsets = [0]
    for i, obj in enumerate(objects, 1):
        offsets.append(f.tell()); f.write(f'{i} 0 obj\n'.encode()+obj+b'\nendobj\n')
    xref = f.tell(); f.write(b'xref\n0 6\n0000000000 65535 f \n')
    for offset in offsets[1:]: f.write(f'{offset:010d} 00000 n \n'.encode())
    f.write(f'trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF'.encode())
    return f.getvalue()


def main():
    env = dict(line.split('=', 1) for line in Path('.env').read_text(encoding='utf-8-sig').splitlines() if '=' in line and not line.startswith('#'))
    emb = request('http://127.0.0.1:8090/v1/embeddings',
        {'model': env['EMBEDDING_MODEL'], 'dimensions': 768, 'input': ['虚拟内存提供地址隔离。', '虚拟内存。' * 200]}, env['EMBEDDING_API_KEY'])
    assert len(emb['data']) == 2
    for item in emb['data']:
        assert len(item['embedding']) == 768
        assert abs(sum(x*x for x in item['embedding'])-1) < 0.001
    print('PASS: local inference, long input, normalized 768-dimensional vectors', flush=True)
    api = 'http://127.0.0.1:8080/api/v1'
    suffix = secrets.token_hex(4)
    auth = request(api+'/auth/register', {'username': 'embedding_test_'+suffix, 'email': suffix+'@example.invalid', 'password': secrets.token_urlsafe(24)})
    token = auth['token']
    kb = request(api+'/knowledge-bases', {'name': 'Local embedding PDF test '+suffix}, token)
    boundary = 'test'+suffix
    multipart = ('--'+boundary+'\r\nContent-Disposition: form-data; name="file"; filename="virtual-memory.pdf"\r\nContent-Type: application/pdf\r\n\r\n').encode()+pdf_bytes()+('\r\n--'+boundary+'--\r\n').encode()
    doc = request(api+'/knowledge-bases/'+kb['ID']+'/documents', multipart, token, 'multipart/form-data; boundary='+boundary)
    for _ in range(40):
        docs = request(api+'/knowledge-bases/'+kb['ID']+'/documents', token=token)
        current = next(d for d in docs if d['ID'] == doc['ID'])
        if current['Status'] == 'ready': break
        if current['Status'] == 'failed': raise RuntimeError(current)
        time.sleep(1)
    else: raise RuntimeError('document processing timeout')
    print('PASS: PDF upload -> parse -> real embedding -> ready; document='+doc['ID'], flush=True)
    result = request(api+'/exam408/assistant/chat', {'KnowledgeBaseID': kb['ID'], 'Question': 'Virtual memory'}, token)
    assert result.get('Sources'), result
    assert result.get('Answer'), result
    print('PASS: retrieval and configured LLM response: '+json.dumps(result, ensure_ascii=True), flush=True)
    chinese = '虚拟内存为进程提供独立的虚拟地址空间，实现地址隔离。程序不必一次全部装入物理内存；操作系统根据需要将页面调入内存，并借助磁盘保存暂时不用的页面。'
    multipart = ('--'+boundary+'\r\nContent-Disposition: form-data; name="file"; filename="virtual-memory-zh.txt"\r\nContent-Type: text/plain\r\n\r\n').encode()+chinese.encode()+('\r\n--'+boundary+'--\r\n').encode()
    zh = request(api+'/knowledge-bases/'+kb['ID']+'/documents', multipart, token, 'multipart/form-data; boundary='+boundary)
    for _ in range(40):
        docs = request(api+'/knowledge-bases/'+kb['ID']+'/documents', token=token)
        current = next(d for d in docs if d['ID'] == zh['ID'])
        if current['Status'] == 'ready': break
        if current['Status'] == 'failed': raise RuntimeError(current)
        time.sleep(1)
    else: raise RuntimeError('Chinese document processing timeout')
    result = request(api+'/exam408/assistant/chat', {'KnowledgeBaseID': kb['ID'], 'Question': '为什么需要虚拟内存？'}, token)
    assert any(s['DocumentID'] == zh['ID'] for s in result.get('Sources', [])), result
    print('PASS: Chinese retrieval and LLM answer: '+json.dumps(result, ensure_ascii=True), flush=True)
    for payload, key, status in [
        ({'model': env['EMBEDDING_MODEL'], 'input': 'test'}, 'wrong-key', 401),
        ({'model': env['EMBEDDING_MODEL'], 'input': 'test', 'dimensions': 1536}, env['EMBEDDING_API_KEY'], 400),
    ]:
        try: request('http://127.0.0.1:8090/v1/embeddings', payload, key)
        except urllib.error.HTTPError as error: assert error.code == status
        else: raise AssertionError('invalid request accepted')
    print('PASS: authentication and dimension rejection', flush=True)


if __name__ == '__main__':
    main()
