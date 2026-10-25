<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, useId, watch } from 'vue'
import type { GraphNode, GraphEdge, KnowledgePoint } from '@/api/types'
import { createGraphLayout, graphGroups, graphSubset, relationLabels, stepGraphLayout, subjectGroups, type LayoutNode } from '@/lib/graphLayout'
const props = defineProps<{ nodes: GraphNode[]; edges: GraphEdge[]; points: KnowledgePoint[] }>()
const emit = defineEmits<{ select: [id: string] }>()
const query = ref(''), selected = ref(''), subject = ref(-1), scale = ref(1), pan = ref({ x: 0, y: 0 }), svg = ref<SVGSVGElement>()
const showNames = ref(true), showRelations = ref(true), neighborOnly = ref(false)
const arrowID = 'knowledge-arrow-' + useId().replace(/:/g, '')
const groups = computed(() => subjectGroups(props.nodes, props.edges, props.points))
const subset = computed(() => graphSubset(props.nodes, props.edges, groups.value, query.value, subject.value))
const adjacent = computed(() => {
  const ids = new Set([selected.value])
  for (const e of props.edges) {
    if (e.FromEntityID === selected.value) ids.add(e.ToEntityID)
    if (e.ToEntityID === selected.value) ids.add(e.FromEntityID)
  }
  return ids
})
const visible = computed(() => neighborOnly.value && selected.value ? subset.value.nodes.filter(n => adjacent.value.has(n.ID)) : subset.value.nodes)
const visibleIDs = computed(() => new Set(visible.value.map(n => n.ID)))
const links = computed(() => subset.value.edges.filter(e => visibleIDs.value.has(e.FromEntityID) && visibleIDs.value.has(e.ToEntityID)))
const counts = computed(() => graphGroups.map((_, i) => props.nodes.filter(n => groups.value.get(n.ID) === i).length))
const width = computed(() => Math.max(1300, Math.sqrt(visible.value.length) * 80))
const height = computed(() => Math.max(860, width.value * .68))
const positions = shallowRef(new Map<string, LayoutNode>())
let frame = 0
let drag: { id?: string; pointer: number; startX: number; startY: number; x: number; y: number; moved: boolean } | null = null
function stop() { cancelAnimationFrame(frame) }
function layout() {
  stop(); drag = null
  const nodes = createGraphLayout(visible.value, links.value, groups.value, width.value, height.value)
  const edges = links.value
  let steps = 0
  const tick = () => {
    for (let i = 0; i < 4; i++) stepGraphLayout(nodes, edges, width.value, height.value)
    positions.value = new Map(nodes.map(n => [n.id, { ...n }]))
    steps += 4
    if (steps < 160) frame = requestAnimationFrame(tick)
  }
  positions.value = new Map(nodes.map(n => [n.id, { ...n }]))
  frame = requestAnimationFrame(tick)
}
watch([visible, links], layout, { immediate: true })
watch(subset, s => { if (selected.value && !s.nodes.some(n => n.ID === selected.value)) select('') })
function select(id: string) { selected.value = id === selected.value ? '' : id; emit('select', selected.value) }
function isActive(e: GraphEdge) { return !selected.value || e.FromEntityID === selected.value || e.ToEntityID === selected.value }
function point(event: PointerEvent | WheelEvent) {
  const matrix = svg.value?.getScreenCTM()
  if (!matrix) return { x: 0, y: 0 }
  return new DOMPoint(event.clientX, event.clientY).matrixTransform(matrix.inverse())
}
function begin(event: PointerEvent, id?: string) {
  if (event.button !== 0) return
  stop()
  const p = point(event), n = id ? positions.value.get(id) : null
  drag = { id, pointer: event.pointerId, startX: p.x, startY: p.y, x: n?.x ?? pan.value.x, y: n?.y ?? pan.value.y, moved: false }
  svg.value?.setPointerCapture(event.pointerId)
}
function move(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId) return
  const p = point(event), dx = p.x - drag.startX, dy = p.y - drag.startY
  drag.moved ||= Math.hypot(dx, dy) > 4
  if (drag.id) {
    const n = positions.value.get(drag.id)
    if (n) positions.value = new Map(positions.value).set(n.id, { ...n, x: drag.x + dx / scale.value, y: drag.y + dy / scale.value })
  } else pan.value = { x: drag.x + dx, y: drag.y + dy }
}
function end(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId) return
  if (event.type !== 'pointercancel' && !drag.moved && drag.id) select(drag.id)
  drag = null
  if (svg.value?.hasPointerCapture(event.pointerId)) svg.value.releasePointerCapture(event.pointerId)
}
function zoom(factor: number, center = { x: width.value / 2, y: height.value / 2 }) {
  const next = Math.max(.35, Math.min(3, scale.value * factor)), ratio = next / scale.value
  pan.value = { x: center.x - (center.x - pan.value.x) * ratio, y: center.y - (center.y - pan.value.y) * ratio }
  scale.value = next
}
function wheel(event: WheelEvent) { zoom(Math.exp(-event.deltaY * .001), point(event)) }
function reset() { query.value = ''; subject.value = -1; selected.value = ''; neighborOnly.value = false; pan.value = { x: 0, y: 0 }; scale.value = 1; emit('select', ''); layout() }
function linePoints(e: GraphEdge) {
  const a = positions.value.get(e.FromEntityID), b = positions.value.get(e.ToEntityID)
  if (!a || !b) return {}
  const dx = b.x - a.x, dy = b.y - a.y, distance = Math.hypot(dx, dy) || 1
  return { x1: a.x + dx / distance * (a.radius + 2), y1: a.y + dy / distance * (a.radius + 2),
    x2: b.x - dx / distance * (b.radius + 5), y2: b.y - dy / distance * (b.radius + 5) }
}
function labelTransform(e: GraphEdge) {
  const a = positions.value.get(e.FromEntityID), b = positions.value.get(e.ToEntityID)
  if (!a || !b) return ''
  let angle = Math.atan2(b.y - a.y, b.x - a.x) * 180 / Math.PI
  if (angle > 90 || angle < -90) angle += 180
  return `translate(${(a.x + b.x) / 2},${(a.y + b.y) / 2}) rotate(${angle})`
}
onBeforeUnmount(stop)
</script>

<template>
  <section class="graph-panel">
    <div class="graph-heading"><div><h2>408 知识关联图</h2><p>四科分色 · 知识聚类 · 来源可追溯</p></div><div class="graph-count"><strong>{{ nodes.length }}</strong> 节点 <span>·</span> <strong>{{ edges.length }}</strong> 关系</div></div>
    <div class="graph-tools"><input v-model="query" type="search" aria-label="搜索知识点" placeholder="搜索知识点，保留一层关联节点…" /><div class="graph-zoom"><button aria-label="缩小图谱" @click="zoom(.8)">−</button><span>{{ Math.round(scale * 100) }}%</span><button aria-label="放大图谱" @click="zoom(1.25)">＋</button></div><button class="secondary" @click="reset">重置视图</button></div>
    <div class="graph-legend" aria-label="按学科筛选"><button :class="{ chosen: subject === -1 }" @click="subject = -1">全部学科</button><button v-for="(g, i) in graphGroups" :key="g.name" :class="{ chosen: subject === i }" :aria-pressed="subject === i" @click="subject = subject === i ? -1 : i"><i :style="{ background: g.color }"></i>{{ g.name }} <small>{{ counts[i] }}</small></button></div>
    <div class="graph-options"><label><input v-model="showNames" type="checkbox" />节点名称</label><label><input v-model="showRelations" type="checkbox" />关系文字</label><label><input v-model="neighborOnly" type="checkbox" :disabled="!selected" />只看选中节点及邻居</label><span>拖动节点调整位置 · 拖动空白处平移 · 滚轮缩放</span></div>
    <div class="graph-workspace"><div class="graph-stage">
      <svg ref="svg" :viewBox="`0 0 ${width} ${height}`" role="group" aria-label="408交互知识图谱" @wheel.prevent="wheel" @pointerdown="begin($event)" @pointermove="move" @pointerup="end" @pointercancel="end">
        <defs><marker :id="arrowID" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto"><path d="M 0 0 L 10 5 L 0 10 z" fill="#b3bfce" /></marker></defs>
        <g :transform="`translate(${pan.x},${pan.y}) scale(${scale})`">
          <g v-for="e in links" :key="e.ID" :opacity="isActive(e) ? 1 : .08">
            <line v-bind="linePoints(e)" :stroke="selected && isActive(e) ? '#7c96bc' : '#d8dee7'" :stroke-width="selected && isActive(e) ? 2.2 : 1.3" :marker-end="e.RelationType === 'related' ? undefined : `url(#${arrowID})`"><title>{{ relationLabels[e.RelationType] || e.RelationType }}{{ e.Source === 'rule_candidate' ? '（规则共现，非因果）' : '' }}</title></line>
            <text v-if="showRelations" :transform="labelTransform(e)" text-anchor="middle" y="-5" class="edge-label">{{ relationLabels[e.RelationType] || e.RelationType }}</text>
          </g>
          <g v-for="n in visible" :key="n.ID" :transform="`translate(${positions.get(n.ID)?.x || 0},${positions.get(n.ID)?.y || 0})`" :opacity="!selected || adjacent.has(n.ID) ? 1 : .16" role="button" tabindex="0" :aria-label="`${n.Name}，点击查看关系与来源`" :aria-pressed="selected === n.ID" class="graph-node" @pointerdown.stop="begin($event, n.ID)" @keydown.enter.prevent="select(n.ID)" @keydown.space.prevent="select(n.ID)">
            <title>{{ n.Name }} · {{ graphGroups[groups.get(n.ID) ?? 4]?.name }}</title>
            <circle v-if="selected === n.ID" :r="(positions.get(n.ID)?.radius || 12) + 7" fill="none" :stroke="graphGroups[groups.get(n.ID) ?? 4]?.color" stroke-width="2" opacity=".4" />
            <circle :r="positions.get(n.ID)?.radius || 12" :fill="graphGroups[groups.get(n.ID) ?? 4]?.color" stroke="white" stroke-width="2.5" />
            <text v-if="showNames || selected === n.ID" :x="(positions.get(n.ID)?.radius || 12) + 7" y="4.5" class="node-label">{{ n.Name.length > 18 ? n.Name.slice(0, 18) + '…' : n.Name }}</text>
          </g>
        </g>
      </svg>
      <p v-if="!visible.length" class="graph-empty">{{ nodes.length ? '没有匹配的知识点，请调整搜索或学科筛选。' : '当前还没有图谱节点，请先处理资料或维护知识点目录。' }}</p>
      <div class="graph-status">当前显示 {{ visible.length }} 节点 · {{ links.length }} 关系<span v-if="subset.total > 600"> · 匹配超过 600 个，请搜索或按学科缩小范围</span></div>
    </div><aside class="graph-details" aria-label="知识点详情与证据"><slot name="details"><h3>探索知识关联</h3><p>点击节点查看知识点说明、相邻关系和资料出处。</p></slot><p class="group-note">科目颜色依据规范目录；资料候选按最近的目录关联分组，无法唯一归科的节点显示灰色。规则共现不代表因果或先修关系。</p></aside></div>
  </section>
</template>

<style scoped>
.graph-panel{background:#fff;border:1px solid #e4eaf2;border-radius:18px;overflow:hidden;margin:20px 0}.graph-heading{display:flex;justify-content:space-between;align-items:center;gap:16px;padding:22px 24px 12px}.graph-heading h2{margin:0;font-size:21px}.graph-heading p{margin:7px 0 0;color:#7a889d;font-size:13px}.graph-count{white-space:nowrap;font-size:13px;color:#708198}.graph-count strong{font-size:23px;color:#23364e;margin:0 5px}.graph-count span{margin:0 7px}.graph-tools{display:flex;gap:12px;padding:10px 24px}.graph-tools>input{flex:1;min-width:100px;border:1px solid #dce3ed;border-radius:9px;padding:10px 12px}.graph-zoom{display:flex;align-items:center;border:1px solid #dce3ed;border-radius:9px}.graph-zoom button{background:transparent;border:0;padding:8px 12px;font-size:18px;cursor:pointer}.graph-zoom span{font-size:12px;min-width:42px;text-align:center}.graph-legend{display:flex;gap:8px;flex-wrap:wrap;padding:8px 24px}.graph-legend button{display:flex;align-items:center;gap:7px;border:1px solid transparent;background:#f7f9fc;border-radius:20px;padding:7px 11px;font-size:12px;color:#53657d;cursor:pointer}.graph-legend .chosen{border-color:#99b9e5;background:#edf4ff;color:#244e87}.graph-legend i{width:10px;height:10px;border-radius:50%}.graph-legend small{color:#8693a5}.graph-options{display:flex;gap:16px;flex-wrap:wrap;align-items:center;font-size:12px;color:#65758b;padding:12px 24px 17px}.graph-options label{display:flex;align-items:center;gap:5px}.graph-options span{margin-left:auto;color:#94a0af}.graph-workspace{display:grid;grid-template-columns:minmax(0,1fr) 290px;border-top:1px solid #eaf0f7}.graph-stage{position:relative;background:#fff;min-height:600px;overflow:hidden}.graph-stage svg{display:block;width:100%;height:650px;touch-action:none;cursor:grab;user-select:none}.graph-stage svg:active{cursor:grabbing}.graph-node{cursor:pointer;outline:none}.graph-node:focus-visible circle{stroke:#172c49;stroke-width:4}.node-label{font-size:17px;fill:#3a485c;paint-order:stroke;stroke:white;stroke-width:3px;stroke-linejoin:round;pointer-events:none}.edge-label{font-size:14px;fill:#a2adbb;paint-order:stroke;stroke:white;stroke-width:3px;pointer-events:none}.graph-status{position:absolute;bottom:0;left:0;right:0;padding:12px 18px;background:#ffffffee;color:#8b98a9;font-size:12px;pointer-events:none;border-top:1px solid #f1f4f8}.graph-details{border-left:1px solid #eaf0f7;background:#fbfcfe;padding:22px 18px;max-height:650px;overflow:auto;color:#42536b;font-size:13px}.group-note{margin-top:24px;border-top:1px solid #e4eaf2;padding-top:16px;color:#8b98a9;font-size:11px;line-height:1.8}.graph-empty{position:absolute;top:42%;left:8%;right:8%;text-align:center;color:#8997aa;pointer-events:none}@media(max-width:1050px){.graph-workspace{grid-template-columns:1fr}.graph-details{border-left:0;border-top:1px solid #eaf0f7;max-height:380px}.graph-stage svg{height:580px}}@media(max-width:650px){.graph-heading{align-items:flex-start;flex-direction:column;padding:18px 16px 8px}.graph-tools,.graph-legend,.graph-options{padding-left:16px;padding-right:16px}.graph-options span{margin-left:0}.graph-tools{flex-wrap:wrap}.graph-stage,.graph-stage svg{height:500px;min-height:500px}}
</style>
