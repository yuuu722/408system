from pathlib import Path
import json
import pypdfium2 as pdfium
from pypdf import PdfReader
root=Path(r'D:\school\科创\平台数据集\组会考研408资料')
out=Path(__file__).parent
targets=[
('os_questions',next(root.rglob('【速刷】26王道操作系统选择题做题本.pdf')),5),
('exam_2025',next(root.rglob('2025年计算机408统考真题.pdf')),0),
('syllabus',next(root.rglob('2026年计算机学科专业基础考试大纲.pdf')),5),
('ds_textbook',next(root.rglob('07.《数据结构》（C语言版） (严蔚敏 吴伟民) .pdf')),173),
]
for name,p,i in targets:
    d=pdfium.PdfDocument(str(p)); page=d[i]
    page.render(scale=1.35).to_pil().save(out/(name+'.png'))
extra=[]
for pattern in ['2026操作系统.pdf','2023年计算机408统考真题.pdf','2023年计算机408统考真题解析.pdf','2022年计算机408统考真题.pdf','2015年计算机408统考真题.pdf','2009-2023 真题与解析打印版408.pdf','【速刷】26王道数据结构选择题做题本.pdf','2027数据结构_高清带书签版.pdf']:
    p=next(root.rglob(pattern)); d=pdfium.PdfDocument(str(p)); indices=sorted(set([min(5,len(d)-1),len(d)//2]))
    row={'path':str(p.relative_to(root)),'pages':len(d),'samples':[]}
    for i in indices:
        t=d[i].get_textpage().get_text_range()
        row['samples'].append({'page':i+1,'chars':len(t),'preview':t[:1000]})
    extra.append(row)
(out/'extra.json').write_text(json.dumps(extra,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(extra,ensure_ascii=False))
