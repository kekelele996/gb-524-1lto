export type MaintenanceWindowStatus = 'upcoming' | 'active' | 'ended'

export interface MaintenanceWindow {
  id: number
  station_id: number
  start_at: string
  end_at: string
  reason: string
  created_by: number
  created_at: string
  updated_at: string
  station?: {
    id: number
    station_code: string
    name: string
  }
  status: MaintenanceWindowStatus
}

export interface MaintenanceWindowInput {
  start_at: string
  end_at: string
  reason: string
}
