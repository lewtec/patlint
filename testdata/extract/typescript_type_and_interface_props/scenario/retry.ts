export interface RetryOptions { shouldRetry?: (error: unknown) => boolean }
export type MockServices = { http: { fetch: number } }
export type SearchResult = { link: string; source: string }
export class DefaultErrorHandler { constructor(app: number) {} }
