import { create } from 'zustand'
import { maintenanceApi } from '../api/maintenance'
import type { MaintenanceWindow, MaintenanceWindowInput } from '../types/maintenance'

interface MaintenanceState {
  windows: MaintenanceWindow[]
  busy: boolean
  load: (stationId?: number) => Promise<void>
  createWindow: (stationId: number, input: MaintenanceWindowInput) => Promise<MaintenanceWindow>
  updateWindow: (id: number, input: MaintenanceWindowInput) => Promise<MaintenanceWindow>
}

export const useMaintenanceStore = create<MaintenanceState>((set, get) => ({
  windows: [],
  busy: false,
  load: async (stationId) => {
    set({ busy: true })
    try {
      const response = await maintenanceApi.list(stationId)
      set({ windows: response.data })
    } finally {
      set({ busy: false })
    }
  },
  createWindow: async (stationId, input) => {
    const response = await maintenanceApi.create(stationId, input)
    set({ windows: [response.data, ...get().windows] })
    return response.data
  },
  updateWindow: async (id, input) => {
    const response = await maintenanceApi.update(id, input)
    set({ windows: get().windows.map((item) => item.id === id ? response.data : item) })
    return response.data
  }
}))
