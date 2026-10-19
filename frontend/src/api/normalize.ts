import type { Dashboard } from './types.ts'

// Empty Go slices are currently serialized as null. Views always receive arrays.
export function asList<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
}
export function normalizeDashboard(value: Dashboard): Dashboard {
  return { ...value, Subjects: asList(value.Subjects), Weaknesses: asList(value.Weaknesses),
    Recommendations: asList(value.Recommendations), RecentDocuments: asList(value.RecentDocuments) }
}
