import test from 'node:test'
import assert from 'node:assert/strict'
import { normalizeAsset, editableAsset } from '../src/lib/assetData.ts'
test('null options and legacy payloads render without approving incomplete items', () => {
 for (const Payload of [null, {}, {Options:null}, {Options:['甲',null]}]) {
  const a=normalizeAsset({ID:'a',Kind:'question',Status:'unavailable',Payload})
  assert.doesNotThrow(()=>a.Payload.Options.map(String))
  assert.equal(a.Status,'unavailable')
  assert.equal(a.Payload.Answer,-1)
 }
 const a=editableAsset({Kind:'question',Payload:{Options:null}})
 assert.deepEqual(a.Payload.Options,['','','',''])
 assert.equal(a.Status,'unavailable')
})
