import {readFileSync,readdirSync,mkdirSync,writeFileSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import path from 'node:path'
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..'),modules=path.join(root,'frontend/node_modules')
const names=['markdown-it','katex','dompurify','pdfjs-dist','argparse','entities','linkify-it','mdurl','punycode.js','uc.micro','commander']
let output='408 Assistant: notices for added answer rendering and PDF dependencies. These libraries are third-party contributions.\n\n'
for(const name of names){const directory=path.join(modules,name),pkg=JSON.parse(readFileSync(path.join(directory,'package.json'),'utf8'));output+=`${name} ${pkg.version} (${pkg.license})\n`;for(const file of readdirSync(directory).filter(f=>/^license(?:[-.].*)?$/i.test(f)))output+=`\n${file}\n${readFileSync(path.join(directory,file),'utf8')}\n`;output+='\n'}
mkdirSync(path.join(root,'frontend/public'),{recursive:true});writeFileSync(path.join(root,'frontend/public/third-party-notices.txt'),output)
