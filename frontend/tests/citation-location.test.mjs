import test from 'node:test'
import assert from 'node:assert/strict'
import {citationTextIndex,citationPDFPage} from '../src/lib/citationLocation.ts'
const pages=[{Number:1,Segment:0,Text:'教材封面与出版社信息。'},{Number:37,Segment:36,Text:'在同一进程中，线程的切换不会引起进程的切换。'}]
test('text fallback locates citation before the file can load',()=>{assert.equal(citationTextIndex(pages,{Page:37,Excerpt:'线程的切换'}),1)})
test('missing page uses a unique quotation, never an implicit cover fallback',()=>{assert.equal(citationTextIndex(pages,{Excerpt:'在同一进程中，线程的切换不会引起进程的切换。'}),1);assert.equal(citationTextIndex(pages,{Excerpt:'未能定位的引用'}),-1)})
test('PDF does not silently clamp an invalid citation to another page',()=>{assert.equal(citationPDFPage({Page:300,Regions:[]},[], -1,110),-1);assert.equal(citationPDFPage({Page:37},pages,1,110),37)})

test('a unique complete quotation repairs stale page metadata',()=>{const s={Page:1,Excerpt:'在同一进程中，线程的切换不会引起进程的切换。'};assert.equal(citationTextIndex(pages,s),1);assert.equal(citationPDFPage(s,pages,1,110),37)})
