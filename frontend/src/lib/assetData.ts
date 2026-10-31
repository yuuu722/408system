import type { Asset } from '../api/foundation'

const text = (value: unknown) => typeof value === 'string' ? value : ''
const number = (value: unknown, fallback: number) => typeof value === 'number' && Number.isFinite(value) ? value : fallback

// Legacy/failed extractions can legitimately contain null options. Normalize
// rendering only; never turn an incomplete question into an approved question.
export function normalizeAsset(value: unknown): Asset {
  const a = value && typeof value === 'object' ? value as Record<string, unknown> : {}
  const p = a.Payload && typeof a.Payload === 'object' ? a.Payload as Record<string, unknown> : {}
  return {
    ...a, ID: text(a.ID), Kind: a.Kind === 'video' || a.Kind === 'material' ? a.Kind : 'question',
    IsPublic: a.IsPublic === true, PointID: text(a.PointID), Title: text(a.Title),
    Status: text(a.Status) || 'unavailable', Revision: number(a.Revision, 0),
    Payload: {
      ...p, Stem: text(p.Stem), Options: Array.isArray(p.Options) ? p.Options.map(text) : [],
      Answer: number(p.Answer, -1), Explanation: text(p.Explanation), Source: text(p.Source),
      URL: text(p.URL), DocumentID: text(p.DocumentID), Page: number(p.Page, 0),
      StartSeconds: number(p.StartSeconds, 0), EndSeconds: number(p.EndSeconds, 0), Difficulty: number(p.Difficulty, 1),
    },
  } as Asset
}
export function editableAsset(value: unknown): Asset {
  const a = normalizeAsset(JSON.parse(JSON.stringify(value)))
  if (a.Kind === 'question' && a.Payload.Options.length < 2) {
    while (a.Payload.Options.length < 4) a.Payload.Options.push('')
  }
  return a
}
