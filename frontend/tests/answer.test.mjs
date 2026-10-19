import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { rotateBox,validBox } from '../src/lib/sourceGeometry.ts'
const dom=new JSDOM('<!doctype html><html><body></body></html>');globalThis.window=dom.window;globalThis.document=dom.window.document
const {renderAnswer}=await import('../src/lib/answer.ts')
function parsed(text,n=1){const root=document.createElement('div');root.innerHTML=renderAnswer(text,n);return root}
test('rich answer renders tables formulas and code',()=>{const root=parsed('# 标题\n\n|A|B|\n|-|-|\n|1|2|\n\n$x^2$\n\n```go\nfmt.Println(1)\n```');assert.ok(root.querySelector('table'));assert.ok(root.querySelector('.katex'));assert.ok(root.querySelector('pre code'))})
test('only valid citations outside code are interactive',()=>{const root=parsed('[1] [9] `[1]`\n\n```\n[1]\n```');assert.equal(root.querySelectorAll('button[data-citation]').length,1);assert.equal(root.querySelector('button').dataset.citation,'1');assert.ok(root.textContent.includes('[9]'))})
test('untrusted output cannot create active html or remote images',()=>{const root=parsed('<script>alert(1)</script><img src=x onerror=alert(1)>\n\n![image](https://example.invalid/image)\n\n[x](javascript:alert(1))\n\n$\\href{javascript:alert(1)}{x}$');assert.equal(root.querySelectorAll('script,img,iframe,[onerror],a[href^="javascript:"]').length,0)})
test('partial streamed markup remains renderable',()=>{assert.doesNotThrow(()=>parsed('$$ \\frac{a}{'));assert.doesNotThrow(()=>parsed('```python\nprint('));assert.doesNotThrow(()=>parsed('| A |'))})
test('highlight boxes rotate without losing bounds',()=>{const box=[.1,.2,.4,.6];let transformed=box;for(let n=0;n<4;n++)transformed=rotateBox(transformed,90);transformed.forEach((v,i)=>assert.ok(Math.abs(v-box[i])<1e-8));assert.equal(validBox([0,0,1,1]),true);assert.equal(validBox([.5,0,.2,1]),false);assert.equal(validBox([NaN,0,1,1]),false)})
