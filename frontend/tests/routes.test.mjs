import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
const router=readFileSync(new URL('../src/router/index.ts',import.meta.url),'utf8')
test('phase-one routes are registered',()=>{for(const path of ['/exam408','exam408/knowledge','exam408/resources','exam408/assistant','exam408/weakness','exam408/settings','/login'])assert.ok(router.includes(path),`missing ${path}`)})
test('protected navigation checks auth token',()=>{assert.match(router,/exam408_token/);assert.match(router,/beforeEach/)})

