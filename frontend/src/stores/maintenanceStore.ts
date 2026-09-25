import { create } from 'zustand'
import { maintenanceApi } from '../api/maintenance'
import type { MaintenanceWindow, MaintenanceWindowInput, MaintenanceWindowUpdateInput } from '../types/maintenance'

interface MaintenanceState {
  windows: MaintenanceWindow[]
  busy: boolean
  load: (stationId?: number) => Promise<void>
  register: (input: MaintenanceWindowInput) => Promise<MaintenanceWindow>
  update: (id: number, input: MaintenanceWindowUpdateInput) => Promise<MaintenanceWindow>
}

function sortWindows(items: MaintenanceWindow[]): MaintenanceWindow[] {
  return [...items].sort((a, b) => b.start_at.localeCompare(a.start_at))
}

export const useMaintenanceStore = create<MaintenanceState>((set, get) => ({
  windows: [],
  busy: false,
  load: async (stationId) => {
    set({ busy: true })
    try {
      const response = await maintenanceApi.list(stationId)
      set({ windows: sortWindows(response.data) })
    } finally {
      set({ busy: false })
    }
  },
  register: async (input) => {
    const response = await maintenanceApi.register(input)
    set({ windows: sortWindows([response.data, ...get().windows]) })
    return response.data
  },
  update: async (id, input) => {
    const response = await maintenanceApi.update(id, input)
    set({ windows: sortWindows(get().windows.map((item) => item.id === id ? response.data : item)) })
    return response.data
  }
}))
