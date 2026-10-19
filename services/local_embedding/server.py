"""Local-only text2vec adapter. Start manually; no model download or auto-start.

Long inputs use normalized mean pooling over token windows (contract v1).
This avoids silent truncation but still needs retrieval-quality evaluation.
"""
import argparse
import hmac
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

os.environ['HF_HUB_OFFLINE'] = '1'
os.environ['TRANSFORMERS_OFFLINE'] = '1'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--model-path', required=True)
    parser.add_argument('--port', type=int, default=8090)
    args = parser.parse_args()
    key = os.environ.get('LOCAL_EMBEDDING_API_KEY', '')
    if not key:
        raise SystemExit('LOCAL_EMBEDDING_API_KEY is required')
    import numpy as np
    import torch
    from sentence_transformers import SentenceTransformer
    torch.set_num_threads(4)
    model = SentenceTransformer(args.model_path, device='cpu', local_files_only=True)
    name = 'text2vec-base-chinese-windowed-v1'
    width = model.max_seq_length - model.tokenizer.num_special_tokens_to_add(pair=False)
    if model.get_sentence_embedding_dimension() != 768 or width < 1:
        raise RuntimeError('Unexpected local model contract')

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass  # Do not log user documents or authorization headers.

        def reply(self, status, body):
            data = json.dumps(body, ensure_ascii=False).encode('utf-8')
            self.send_response(status)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            self.reply(200 if self.path == '/healthz' else 404,
                       {'model': name, 'dimension': 768, 'device': 'cpu'})

        def do_POST(self):
            if self.path != '/v1/embeddings':
                return self.reply(404, {'error': 'not found'})
            if not hmac.compare_digest(self.headers.get('Authorization', ''), 'Bearer ' + key):
                return self.reply(401, {'error': 'unauthorized'})
            try:
                size = int(self.headers.get('Content-Length', '0'))
                if not 0 < size <= 4 * 1024 * 1024:
                    return self.reply(413, {'error': 'request too large'})
                body = json.loads(self.rfile.read(size))
                texts = body['input']
                if isinstance(texts, str):
                    texts = [texts]
                if body.get('model') != name or body.get('dimensions', 768) != 768:
                    raise ValueError()
                if not isinstance(texts, list) or not 0 < len(texts) <= 4096:
                    raise ValueError()
                if any(not isinstance(t, str) or not t.strip() for t in texts):
                    raise ValueError()
                vectors = []
                for text in texts:
                    ids = model.tokenizer.encode(text, add_special_tokens=False)
                    windows = [model.tokenizer.decode(ids[i:i+width]) for i in range(0, len(ids), width)]
                    encoded = model.encode(windows, batch_size=16, normalize_embeddings=True, show_progress_bar=False)
                    vector = np.mean(encoded, axis=0)
                    norm = np.linalg.norm(vector)
                    if not np.isfinite(norm) or norm <= 0:
                        raise ValueError()
                    vectors.append((vector / norm).tolist())
                self.reply(200, {'object': 'list', 'model': name, 'data': [
                    {'object': 'embedding', 'index': i, 'embedding': v} for i, v in enumerate(vectors)]})
            except (ValueError, KeyError, TypeError):
                self.reply(400, {'error': 'invalid embedding request'})
            except Exception:
                self.reply(500, {'error': 'local embedding inference failed'})

    print(f'Loaded {name}; CPU; port {args.port}', flush=True)
    HTTPServer(('0.0.0.0', args.port), Handler).serve_forever()


if __name__ == '__main__':
    main()
