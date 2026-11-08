import { USER_ID } from '../config'

export class ApiError extends Error {
  constructor(message: string, public readonly status: number) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('X-User-ID', String(USER_ID))
  if (options.body !== undefined) headers.set('Content-Type', 'application/json')

  let response: Response
  try {
    response = await fetch(path, { ...options, headers })
  } catch (error) {
    if (options.signal?.aborted) throw error
    throw new Error('Unable to reach the server. Check your connection and try again.')
  }

  let payload: unknown
  try {
    payload = await response.json()
  } catch (error) {
    if (options.signal?.aborted) throw error
    throw new ApiError(
      response.ok ? 'The server returned an invalid JSON response.' : `Request failed (${response.status}). Please try again.`,
      response.status,
    )
  }

  if (!response.ok) {
    const message = payload && typeof payload === 'object' && 'error' in payload && typeof payload.error === 'string'
      ? payload.error
      : `Request failed (${response.status}). Please try again.`
    throw new ApiError(message, response.status)
  }
  return payload as T
}
