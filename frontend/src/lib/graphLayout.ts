import type { GraphNode, GraphEdge, KnowledgePoint } from '../api/types'

export const graphGroups = [
  { name: '数据结构', color: '#4c97f6' },
  { name: '计算机组成原理', color: '#65bf3e' },
  { name: '操作系统', color: '#e6a638' },
  { name: '计算机网络', color: '#f36b72' },
  { name: '未归科资料概念', color: '#8896ac' },
]
export const relationLabels: Record<string, string> = {
  prerequisite: '先修', related: '相关', part_of: '组成', implements: '实现', uses: '使用', applies_to: '应用于',
}
export function subjectGroups(nodes: GraphNode[], edges: GraphEdge[], points: KnowledgePoint[]) {
  const pointsByID = new Map(points.map(p => [p.ID, p]))
  const groups = new Map<string, number>()
  for (const n of nodes) {
    const subject = pointsByID.get(n.KnowledgePointID)?.SubjectName
    const index = graphGroups.findIndex(g => g.name === subject)
    if (index >= 0 && index < 4) groups.set(n.ID, index)
  }
  // Color unattached candidates by the nearest reachable canonical subject.
  // Multi-source BFS is O(V+E); ambiguous/disconnected candidates stay neutral.
  const adjacent = new Map(nodes.map(n => [n.ID, new Set<string>()]))
  for (const e of edges) {
    adjacent.get(e.FromEntityID)?.add(e.ToEntityID)
    adjacent.get(e.ToEntityID)?.add(e.FromEntityID)
  }
  const distances = new Map<string, number[]>(nodes.map(n => [n.ID, [Infinity, Infinity, Infinity, Infinity]]))
  const queue: [string, number][] = []
  for (const [id, g] of groups) { distances.get(id)![g] = 0; queue.push([id, g]) }
  for (let i = 0; i < queue.length; i++) {
    const [id, g] = queue[i]!
    for (const neighbor of adjacent.get(id) ?? []) {
      const d = distances.get(neighbor)
      if (d && d[g]! > distances.get(id)![g]! + 1) { d[g] = distances.get(id)![g]! + 1; queue.push([neighbor, g]) }
    }
  }
  for (const n of nodes) {
    if (groups.has(n.ID)) continue
    const d = distances.get(n.ID)!, nearest = Math.min(...d)
    groups.set(n.ID, Number.isFinite(nearest) && d.filter(v => v === nearest).length === 1 ? d.indexOf(nearest) : 4)
  }
  return groups
}

export function graphSubset(nodes: GraphNode[], edges: GraphEdge[], groups: Map<string, number>, query: string, group: number, limit = 600) {
  const needle = query.trim().toLocaleLowerCase()
  const matching = nodes.filter(n => (group < 0 || groups.get(n.ID) === group) &&
    (!needle || `${n.Name} ${n.Description}`.toLocaleLowerCase().includes(needle)))
  const matches = new Set(matching.map(n => n.ID))
  const selected = new Set(matches)
  // Keep immediate neighbors to make search results interpretable as a graph.
  if (needle) for (const e of edges) {
    if (selected.has(e.FromEntityID) || selected.has(e.ToEntityID)) {
      if (matches.has(e.FromEntityID)) selected.add(e.ToEntityID)
      if (matches.has(e.ToEntityID)) selected.add(e.FromEntityID)
    }
  }
  const context = nodes.filter(n => selected.has(n.ID) && !matches.has(n.ID) && (group < 0 || groups.get(n.ID) === group))
  let ordered = [...matching, ...context]
  // A bounded overview must still represent all subjects, rather than just
  // whichever subject happens to appear first in the database result.
  if (!needle && group < 0 && ordered.length > limit) {
    const buckets = graphGroups.map((_, i) => ordered.filter(n => (groups.get(n.ID) ?? 4) === i))
    ordered = []
    for (let offset = 0; ordered.length < Math.max(0, limit); offset++) {
      let added = false
      for (const bucket of buckets) if (bucket[offset] && ordered.length < limit) { ordered.push(bucket[offset]!); added = true }
      if (!added) break
    }
  }
  const visible = ordered.slice(0, Math.max(0, limit))
  const ids = new Set(visible.map(n => n.ID))
  return { nodes: visible, edges: edges.filter(e => ids.has(e.FromEntityID) && ids.has(e.ToEntityID)), matches,
    total: matching.length + context.length }
}

export interface LayoutNode { id: string; x: number; y: number; vx: number; vy: number; group: number; radius: number; labelWidth: number }
export function createGraphLayout(nodes: GraphNode[], edges: GraphEdge[], groups: Map<string, number>, width: number, height: number): LayoutNode[] {
  const degrees = new Map<string, number>()
  for (const e of edges) for (const id of [e.FromEntityID, e.ToEntityID]) degrees.set(id, (degrees.get(id) ?? 0) + 1)
  const counters = [0, 0, 0, 0, 0]
  return nodes.map(n => {
    const group = groups.get(n.ID) ?? 4, i = counters[group] ?? 0
    counters[group] = i + 1
    const center = groupCenter(group, width, height), angle = i * 2.399963, radius = 40 * Math.sqrt(i)
    return { id: n.ID, x: Math.max(50, Math.min(width - 120, center.x + Math.cos(angle) * radius)),
      y: Math.max(50, Math.min(height - 50, center.y + Math.sin(angle) * radius)), vx: 0, vy: 0, group,
      radius: 18 + Math.min(12, Math.sqrt(degrees.get(n.ID) ?? 0) * 2.4), labelWidth: Math.min(n.Name.length, 18) * 16 + 30 }
  })
}
function groupCenter(g: number, w: number, h: number) {
  return { x: w * ([.26, .74, .26, .74, .5][g] ?? .5), y: h * ([.25, .25, .75, .75, .5][g] ?? .5) }
}
export function stepGraphLayout(nodes: LayoutNode[], edges: GraphEdge[], width: number, height: number) {
  const byID = new Map(nodes.map(n => [n.id, n]))
  for (let i = 0; i < nodes.length; i++) for (let j = i + 1; j < nodes.length; j++) {
    const a = nodes[i]!, b = nodes[j]!, dx = b.x - a.x || .1, dy = b.y - a.y || .1, d2 = dx * dx + dy * dy, d = Math.sqrt(d2)
    const force = Math.min(4, 1900 / (d2 + 80))
    let fx = dx / d * force, fy = dy / d * force
    const labelGap = (a.labelWidth + b.labelWidth) / 2
    if (Math.abs(dy) < 35 && Math.abs(dx) < labelGap) { fy += Math.sign(dy) * (35 - Math.abs(dy)) * .15; fx += Math.sign(dx) * .4 }
    a.vx -= fx; a.vy -= fy; b.vx += fx; b.vy += fy
  }
  for (const e of edges) {
    const a = byID.get(e.FromEntityID), b = byID.get(e.ToEntityID)
    if (!a || !b || a === b) continue
    const dx = b.x - a.x, dy = b.y - a.y, d = Math.hypot(dx, dy) || 1
    const f = (d - (a.group === b.group ? 150 : 260)) * .012
    a.vx += dx / d * f; a.vy += dy / d * f; b.vx -= dx / d * f; b.vy -= dy / d * f
  }
  for (const n of nodes) {
    const c = groupCenter(n.group, width, height)
    n.vx = (n.vx + (c.x - n.x) * .005) * .72
    n.vy = (n.vy + (c.y - n.y) * .005) * .72
    n.x = Math.max(40, Math.min(width - 170, n.x + Math.max(-10, Math.min(10, n.vx))))
    n.y = Math.max(45, Math.min(height - 45, n.y + Math.max(-10, Math.min(10, n.vy))))
  }
}
