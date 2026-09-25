export interface MaintenanceWindow {
  id: number
  station_id: number
  start_at: string
  end_at: string
  reason: string
  created_by: number
  created_at: string
  updated_at: string
  station?: import('./station').ReceiverStation
}

export interface MaintenanceWindowInput {
  station_id: number
  start_at: string
  end_at: string
  reason: string
}

export interface MaintenanceWindowUpdateInput {
  start_at: string
  end_at: string
  reason: string
}

export interface SkippedStation {
  observation_id: number
  station_id: number
  station_code: string
  reason_code: 'station_in_maintenance' | 'station_not_active'
  reason: string
  maintenance_window_id?: number
  maintenance_start_at?: string
  maintenance_end_at?: string
  maintenance_reason?: string
}
