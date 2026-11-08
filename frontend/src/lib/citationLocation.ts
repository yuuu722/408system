import type {Citation} from '@/api/types'
import type {StructuredPage} from '@/api/foundation'
const normalize=(s:string)=>s.replace(/\\([\\`*_{}[\]()#+.!-])/g,'$1').replace(/\s+/g,'')
export function citationTextIndex(pages:StructuredPage[],source:Citation):number{
 const excerpt=normalize(source.Excerpt),first=normalize(source.Excerpt.split(/[。；\n]/)[0]??'')
 // A unique substantial quotation can repair stale page metadata without
 // guessing a new sentence or defaulting to the cover.
 for(const anchor of [excerpt,first]){if(anchor.length<20)continue;const matches=pages.map((p,i)=>normalize(p.Text).includes(anchor)?i:-1).filter(i=>i>=0);if(matches.length===1)return matches[0]??-1}
 for(const n of [source.Page,...(source.Regions??[]).map(r=>r.Page)]){if(n&&n>0){const i=pages.findIndex(p=>p.Number===n);if(i>=0)return i}}
 for(const r of source.Regions??[]){if(r.BlockID||r.Box?.length||r.End>r.Start){const i=pages.findIndex(p=>(p.Segment??0)===r.Segment);if(i>=0)return i}}

 return -1
}
export function citationPDFPage(source:Citation,pages:StructuredPage[],textIndex:number,count:number):number{
 for(const n of [pages[textIndex]?.Number,source.Page,...(source.Regions??[]).map(r=>r.Page)])if(n&&Number.isInteger(n)&&n>0&&n<=count)return n
 return -1
}
