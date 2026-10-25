import test from 'node:test'
import assert from 'node:assert/strict'
import { subjectGroups, graphSubset, createGraphLayout, stepGraphLayout } from '../src/lib/graphLayout.ts'
const node = (ID, point = '') => ({ ID, Name: ID, Description: '', KnowledgePointID: point })
const edge = (a, b) => ({ ID: a + b, FromEntityID: a, ToEntityID: b, RelationType: 'related' })
const points = [{ ID: 'ds', SubjectName: '数据结构' }, { ID: 'os', SubjectName: '操作系统' }]
test('candidate colors follow nearest canonical subject while ties remain neutral', () => {
  const nodes = [node('a', 'ds'), node('b'), node('c', 'os'), node('alone')]
  const groups = subjectGroups(nodes, [edge('a', 'b'), edge('b', 'c')], points)
  assert.equal(groups.get('a'), 0); assert.equal(groups.get('c'), 2)
  assert.equal(groups.get('b'), 4); assert.equal(groups.get('alone'), 4)
  assert.equal(subjectGroups(nodes, [edge('a', 'b')], points).get('b'), 0)
})
test('search keeps direct neighbors but does not invent relations or expand beyond one hop', () => {
  const nodes = [node('a'), node('b'), node('c'), node('other')], edges = [edge('a', 'b'), edge('b', 'c')]
  const subset = graphSubset(nodes, edges, new Map(), 'a', -1)
  assert.deepEqual(subset.nodes.map(n => n.ID), ['a', 'b'])
  assert.equal(subset.edges.length, 1)
  assert.equal(graphSubset(nodes, edges, new Map(), 'not-found', -1).nodes.length, 0)
})
test('subject and rendering limits never leave dangling edges', () => {
  const nodes = [node('a'), node('b'), node('c')], edges = [edge('a', 'b'), edge('b', 'c')]
  const groups = new Map([['a', 0], ['b', 0], ['c', 2]])
  const s = graphSubset(nodes, edges, groups, '', 0, 1)
  assert.equal(s.nodes.length, 1); assert.equal(s.edges.length, 0); assert.equal(s.total, 2)
})
test('force layout remains finite and bounded with isolated nodes and invalid links', () => {
  const nodes = Array.from({ length: 90 }, (_, i) => node('n' + i))
  const edges = [edge('n0', 'n1'), edge('missing', 'n0'), edge('n0', 'n0')]
  const groups = new Map(nodes.map((n, i) => [n.ID, i % 5]))
  const a = createGraphLayout(nodes, edges, groups, 1300, 900)
  const b = createGraphLayout(nodes, edges, groups, 1300, 900)
  assert.deepEqual(a, b)
  for (let i = 0; i < 160; i++) stepGraphLayout(a, edges, 1300, 900)
  for (const n of a) {
    assert.ok(Number.isFinite(n.x) && Number.isFinite(n.y))
    assert.ok(n.x >= 40 && n.x <= 1130 && n.y >= 45 && n.y <= 855)
  }
  assert.ok(a[0].radius > a[2].radius)
})
test('search can locate nodes beyond the initial 600-node rendering cap', () => {
  const nodes = Array.from({ length: 650 }, (_, i) => node('concept-' + i))
  const edges = [edge('concept-0', 'concept-649')]
  const overview = graphSubset(nodes, edges, new Map(), '', -1)
  assert.equal(overview.nodes.length, 600); assert.equal(overview.total, 650)
  const found = graphSubset(nodes, edges, new Map(), 'concept-649', -1)
  assert.deepEqual(found.nodes.map(n => n.ID), ['concept-649', 'concept-0'])
  assert.equal(found.edges.length, 1)
})
