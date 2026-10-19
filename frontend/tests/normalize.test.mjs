import test from 'node:test'
import assert from 'node:assert/strict'
import { asList, normalizeDashboard } from '../src/api/normalize.ts'

test('new user null collections render as empty lists', () => {
  assert.deepEqual(asList(null), [])
  assert.deepEqual(asList(undefined), [])
  const rows = [{ ID: 'existing', Name: '操作系统' }]
  assert.deepEqual(asList(rows), rows)
})

test('dashboard handles Go nil slices without losing real counts', () => {
  const data = normalizeDashboard({ Subjects: null, Weaknesses: null, Recommendations: null,
    RecentDocuments: null, KnowledgeBaseCount: 2, ReadyDocumentCount: 3 })
  assert.deepEqual(data.Subjects, [])
  assert.deepEqual(data.Weaknesses, [])
  assert.deepEqual(data.Recommendations, [])
  assert.deepEqual(data.RecentDocuments, [])
  assert.equal(data.KnowledgeBaseCount, 2)
  assert.equal(data.ReadyDocumentCount, 3)
})
