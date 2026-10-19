// Static dependency/branding guard, not an originality or visual-similarity audit.
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, relative, extname } from 'node:path'

const root = fileURLToPath(new URL('../', import.meta.url))
const excluded = new Set(['.git', '.cache', '.npm-cache', 'node_modules', 'dist', 'coverage', 'storage'])
const textExtensions = new Set(['.go', '.mod', '.sum', '.ts', '.js', '.mjs', '.vue', '.css', '.html', '.json', '.sql', '.yaml', '.yml', '.conf', '.svg', '.sh', '.ps1'])
const violations = []
let checked = 0
function walk(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name)
    const name = relative(root, path).replaceAll('\\', '/')
    if (entry.isSymbolicLink()) {
      violations.push(`${name}: symlink requires explicit independence review`)
    } else if (entry.isDirectory()) {
      if (!excluded.has(entry.name)) walk(path)
    } else if (entry.isFile()) {
      if (/weknora/i.test(name)) violations.push(`${name}: reference brand in runtime file name`)
      if (textExtensions.has(extname(path)) || ['Dockerfile', 'Makefile', '.env.example'].includes(entry.name)) {
        checked++
        if (/weknora|weknora\.weixin\.qq\.com/i.test(readFileSync(path, 'utf8'))) {
          violations.push(`${name}: reference project in source/dependency/deployment content`)
        }
      }
    }
  }
}
// Documentation and this guard may name the reference. Never read .env or uploads.
for (const folder of ['backend', 'frontend']) walk(resolve(root, folder))
for (const file of readdirSync(root)) {
  if (/^(?:compose.*\.ya?ml|docker-compose.*\.ya?ml|Dockerfile.*|Makefile|\.env\.example)$/.test(file)) {
    checked++
    if (/weknora/i.test(readFileSync(resolve(root, file), 'utf8'))) violations.push(`${file}: reference coupling`)
  }
}
if (violations.length) {
  console.error(violations.join('\n'))
  process.exitCode = 1
} else {
  console.log(`PASS: ${checked} source/dependency/deployment files checked; no explicit reference coupling. Not an originality audit.`)
}
