import { request } from './client'
import type { CreateStrategyRequest, CreateStrategyResponse, StrategyListResponse, TemplatesListResponse } from '../types/strategy'

export function listStrategies(signal?: AbortSignal) {
  return request<StrategyListResponse>('/api/strategies', { signal })
}

export function createStrategy(strategy: CreateStrategyRequest) {
  return request<CreateStrategyResponse>('/api/strategies', { method: 'POST', body: JSON.stringify(strategy) })
}

export function updateStrategy(id: string, strategy: CreateStrategyRequest) {
  return request<CreateStrategyResponse>(`/api/strategies/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(strategy) })
}

export function listTemplates(signal?: AbortSignal) {
  return request<TemplatesListResponse>('/api/templates', { signal }, false)
}
