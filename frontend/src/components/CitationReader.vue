<script setup lang="ts">
import { computed,nextTick,onBeforeUnmount,onMounted,ref,watch } from 'vue'
import type { PDFDocumentProxy,PDFDocumentLoadingTask,RenderTask } from 'pdfjs-dist'
import workerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { api,messageOf } from '@/api/client'
import { foundation,openDocument,type ParsedDocument } from '@/api/foundation'
import type { Citation } from '@/api/types'
import { rotateBox,validBox } from '@/lib/sourceGeometry'
const props=defineProps<{source:Citation|null}>(),emit=defineEmits<{close:[]}>()
const parsed=ref<ParsedDocument|null>(null),error=ref(''),notice=ref(''),busy=ref(false),pdfMode=ref(false),index=ref(0),zoom=ref(1),rotation=ref(0),highlight=ref(true)
const canvas=ref<HTMLCanvasElement>(),surface=ref<HTMLElement>(),size=ref({width:0,height:0})
let pdf:PDFDocumentProxy|null=null,loadingTask:PDFDocumentLoadingTask|null=null,renderTask:RenderTask|undefined,epoch=0,paintEpoch=0
const page=computed(()=>parsed.value?.Pages[index.value])
const total=computed(()=>pdf?.numPages??parsed.value?.Pages.length??0)
const regions=computed(()=>props.source?.Regions?.filter(r=>pdfMode.value?r.Page===index.value+1:r.Segment===(page.value?.Segment??index.value))??[])
const mapped=computed(()=>highlight.value&&page.value?.CoordinateSpace==='normalized-top-left-displayed-page'?regions.value.filter(r=>validBox(r.Box)):[])
function area(box:number[]){const [x0=0,y0=0,x1=0,y1=0]=rotateBox(box,rotation.value);return {left:x0*100+'%',top:y0*100+'%',width:(x1-x0)*100+'%',height:(y1-y0)*100+'%'}}
function cleanup(){renderTask?.cancel();renderTask=undefined;if(loadingTask){void loadingTask.destroy();loadingTask=null};pdf=null}
async function paint(){
 if(!pdf||!canvas.value)return
 const id=++paintEpoch;renderTask?.cancel();error.value=''
 try{const p=await pdf.getPage(index.value+1);if(id!==paintEpoch)return
 const base=p.getViewport({scale:1,rotation:(p.rotate+rotation.value)%360}),width=Math.min(900,Math.max(280,(surface.value?.clientWidth??600)-12))
 const view=p.getViewport({scale:width/base.width*zoom.value,rotation:(p.rotate+rotation.value)%360}),ratio=Math.min(window.devicePixelRatio||1,2),target=canvas.value
 target.width=Math.floor(view.width*ratio);target.height=Math.floor(view.height*ratio);size.value={width:view.width,height:view.height}
 const context=target.getContext('2d');if(!context)return
 renderTask=p.render({canvas:target,canvasContext:context,viewport:view,transform:ratio===1?undefined:[ratio,0,0,ratio,0,0]})
 await renderTask.promise
 if(page.value?.Rotation!==undefined&&page.value.Rotation!==p.rotate)notice.value='页面旋转基准不一致，仅提供页级定位。'
 }catch(e){if(id===paintEpoch&&!(e instanceof Error&&e.name==='RenderingCancelledException'))error.value=messageOf(e)}
}
watch(()=>props.source,async source=>{
 const id=++epoch;paintEpoch++;cleanup();parsed.value=null;error.value='';notice.value='';zoom.value=1;rotation.value=0;highlight.value=true;pdfMode.value=false
 if(!source)return;busy.value=true
 try{
  const structure=await foundation.structure(source.DocumentID,source.RevisionID??'');if(id!==epoch)return;parsed.value=structure
  const file=await api.get(`/documents/${encodeURIComponent(source.DocumentID)}/file`,{responseType:'blob'});if(id!==epoch)return
  if(String(file.headers['content-type']).includes('application/pdf')){
   const pdfjs=await import('pdfjs-dist');pdfjs.GlobalWorkerOptions.workerSrc=workerURL
   const bytes=new Uint8Array(await (file.data as Blob).arrayBuffer());if(id!==epoch)return
   const task=pdfjs.getDocument({data:bytes,enableXfa:false});loadingTask=task;const loaded=await task.promise
   if(id!==epoch){void task.destroy();return};pdf=loaded;pdfMode.value=true;index.value=Math.max(0,Math.min((source.Page??1)-1,pdf.numPages-1))
   await nextTick();await paint()
  }else{index.value=Math.max(0,structure.Pages.findIndex(p=>source.Page&&source.Page>0?p.Number===source.Page:(p.Segment??0)===(source.Regions?.[0]?.Segment??0)))}
  if(!source.Regions?.length)notice.value='该引用尚无精确位置映射，显示页码与历史引用片段。'
  else if(pdfMode.value&&!mapped.value.length)notice.value='当前页面暂无可靠坐标，显示页码与引用片段。'
 }catch(e){if(id===epoch)error.value=messageOf(e)}finally{if(id===epoch)busy.value=false}
},{immediate:true})
watch([index,zoom,rotation],async()=>{if(pdfMode.value){await nextTick();await paint()}})
function onKey(event:KeyboardEvent){if(props.source&&event.key==='Escape')emit('close')}
onMounted(()=>window.addEventListener('keydown',onKey))
onBeforeUnmount(()=>{window.removeEventListener('keydown',onKey);epoch++;paintEpoch++;cleanup()})
</script>
<template><aside v-if="source" class="citation-reader" role="region" aria-label="引用原文阅读器"><header><div><h2>{{source.DocumentTitle}}</h2><small>高亮表示引用证据的位置，请结合上下文复核。</small></div><button class="secondary" @click="emit('close')">关闭原文</button></header><p v-if="busy" role="status">正在加载原文…</p><p v-if="error" class="notice error" role="alert">{{error}}<br/>历史引用片段仍保留；原文可能已删除或该版本不可访问。</p><blockquote>{{source.Excerpt}}</blockquote><p v-if="notice" class="notice">{{notice}}</p><div v-if="parsed" class="toolbar-actions"><button class="secondary" :disabled="index<=0||busy" @click="index--">上一页／段</button><span>{{index+1}} / {{total}}</span><button class="secondary" :disabled="index>=total-1||busy" @click="index++">下一页／段</button><template v-if="pdfMode"><button class="secondary" :disabled="zoom<=0.75" @click="zoom-=0.25">缩小</button><button class="secondary" :disabled="zoom>=2" @click="zoom+=0.25">放大</button><button class="secondary" @click="rotation=(rotation+90)%360">旋转</button></template><button class="secondary" @click="highlight=!highlight">{{highlight?'取消高亮':'显示高亮'}}</button></div><div ref="surface" class="reader-surface"><div v-if="pdfMode" class="pdf-page" :style="{width:size.width+'px',height:size.height+'px'}"><canvas ref="canvas" :style="{width:size.width+'px',height:size.height+'px'}"/><template v-if="!notice.includes('旋转基准')"><span v-for="(r,n) in mapped" :key="n" class="evidence-box" :style="area(r.Box!)" title="引用的证据区域"/></template></div><section v-else-if="page"><p class="muted">解析文本视图（不代表原文件排版）</p><p v-for="w in page.Warnings??[]" :key="w" class="notice">{{w}}</p><template v-if="page.Blocks?.length"><p v-for="b in page.Blocks" :key="b.ID" :class="{'evidence-text':highlight&&regions.some(r=>r.BlockID===b.ID)}" style="white-space:pre-wrap">{{b.Text}}</p></template><pre v-else style="white-space:pre-wrap">{{page.Text}}</pre></section></div><footer><button class="secondary" @click="openDocument(source.DocumentID,source.Page??0).catch(e=>error=messageOf(e))">单独打开原文件</button></footer></aside></template>
<style scoped>
.citation-reader{box-sizing:border-box;position:fixed;z-index:1500;right:0;top:0;height:100dvh;width:min(760px,90vw);background:white;box-shadow:-8px 0 40px #0f172a33;padding:20px;display:flex;flex-direction:column;gap:12px;overflow:auto}.citation-reader header{display:flex;justify-content:space-between;gap:12px}.citation-reader h2{margin:0;font-size:18px}.citation-reader blockquote{margin:0;padding:12px;background:#f1f5f9;white-space:pre-wrap;max-height:150px;overflow:auto}.reader-surface{flex:1;min-height:200px;overflow:auto;background:#e2e8f0;padding:6px}.reader-surface section{padding:16px;background:white}.pdf-page{position:relative}.evidence-box{position:absolute;background:#facc1544;outline:1px solid #eab30899;pointer-events:none}.evidence-text{background:#fef08a;padding:4px}.citation-reader footer{padding-top:8px}.citation-reader .toolbar-actions{display:flex;flex-wrap:wrap;gap:6px;align-items:center}.citation-reader .toolbar-actions button{white-space:nowrap;flex-shrink:0}.citation-reader header button{white-space:nowrap;flex-shrink:0}@media(max-width:700px){.citation-reader{width:100%;padding:12px}}
</style>
