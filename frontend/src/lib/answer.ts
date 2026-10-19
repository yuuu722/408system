import MarkdownIt from 'markdown-it'
import katex from 'katex'
import DOMPurify from 'dompurify'

const markdown = new MarkdownIt({ html: false, breaks: true, linkify: false })
markdown.disable('image')
markdown.validateLink = (url: string) => /^(https?:|mailto:|#)/i.test(url)
markdown.renderer.rules.link_open = (tokens, idx, options, env, self) => {
 tokens[idx]!.attrSet('target','_blank'); tokens[idx]!.attrSet('rel','noopener noreferrer')
 return self.renderToken(tokens,idx,options)
}
markdown.inline.ruler.before('escape','answer_math',(state,silent)=>{
 const src=state.src, pos=state.pos
 const pair=[['$$','$$'],['\\[','\\]'],['\\(','\\)'],['$','$']].find(([open])=>src.startsWith(open!,pos))
 if(!pair)return false
 const [open,close]=pair as [string,string], end=src.indexOf(close,pos+open.length)
 if(end<0||end-pos>4096)return false
 if(!silent){const token=state.push('answer_math','',0);token.content=src.slice(pos+open.length,end);token.meta={display:open==='$$'||open==='\\['}}
 state.pos=end+close.length;return true
})
markdown.renderer.rules.answer_math=(tokens,idx)=>katex.renderToString(tokens[idx]!.content,{displayMode:tokens[idx]!.meta?.display===true,throwOnError:false,trust:false,maxExpand:200,maxSize:10})
markdown.inline.ruler.before('link','answer_citation',(state,silent)=>{
 const match=/^\[(\d{1,3})\]/.exec(state.src.slice(state.pos));if(!match)return false
 const n=Number(match[1]);if(n<1||n>Number(state.env?.citationCount??0))return false
 if(!silent){const token=state.push('answer_citation','',0);token.content=String(n)}
 state.pos+=match[0].length;return true
})
markdown.renderer.rules.answer_citation=(tokens,idx)=>`<button type="button" class="citation-number" data-citation="${tokens[idx]!.content}" aria-label="查看引用 ${tokens[idx]!.content}">[${tokens[idx]!.content}]</button>`
export function renderAnswer(text:string,citationCount:number){
 return DOMPurify.sanitize(markdown.render(text,{citationCount}),{FORBID_TAGS:['img','iframe','form','input','style','svg'],ADD_ATTR:['target','data-citation'],ALLOW_DATA_ATTR:false})
}
