export interface Strategy {
  id: string
  name: string
  description: string
  script: string
  created_at: string
  updated_at: string
}

export interface CreateStrategyRequest { name: string; description?: string; script: string }
export interface CreateStrategyResponse { id: string }
export interface StrategyListResponse { strategies: Strategy[] }
export interface TemplatesListResponse { templates: Strategy[] }
