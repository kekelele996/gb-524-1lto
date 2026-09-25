import { apiClient } from './client'
import type { MaintenanceWindow, MaintenanceWindowInput } from '../types/maintenance'

export const maintenanceApi = {
  list: (stationId?: number) =>
    apiClient.get<MaintenanceWindow[]>(`/maintenance-windows${stationId ? `?station_id=${stationId}` : ''}`),
  create: (stationId: number, input: MaintenanceWindowInput) =>
    apiClient.post<MaintenanceWindow>(`/stations/${stationId}/maintenance-windows`, input),
  update: (id: number, input: MaintenanceWindowInput) =>
    apiClient.put<MaintenanceWindow>(`/maintenance-windows/${id}`, input)
}
