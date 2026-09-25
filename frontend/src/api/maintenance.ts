import { apiClient } from './client'
import type { MaintenanceWindow, MaintenanceWindowInput, MaintenanceWindowUpdateInput } from '../types/maintenance'

export const maintenanceApi = {
  list: (stationId = 0, pageSize = 100) =>
    apiClient.getPage<MaintenanceWindow[]>(`/maintenance-windows?page_size=${pageSize}${stationId ? `&station_id=${stationId}` : ''}`),
  get: (id: number) => apiClient.get<MaintenanceWindow>(`/maintenance-windows/${id}`),
  register: (input: MaintenanceWindowInput) => apiClient.post<MaintenanceWindow>('/maintenance-windows', input),
  update: (id: number, input: MaintenanceWindowUpdateInput) => apiClient.put<MaintenanceWindow>(`/maintenance-windows/${id}`, input)
}
