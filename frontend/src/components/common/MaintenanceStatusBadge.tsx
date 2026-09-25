import BuildRounded from '@mui/icons-material/BuildRounded'
import ScheduleRounded from '@mui/icons-material/ScheduleRounded'
import TaskAltRounded from '@mui/icons-material/TaskAltRounded'
import { Chip } from '@mui/material'
import type { MaintenanceWindowStatus } from '../../types/maintenance'

const statusLabels: Record<MaintenanceWindowStatus, string> = {
  active: '维护中',
  upcoming: '待生效',
  ended: '已结束'
}

const statusColors: Record<MaintenanceWindowStatus, 'warning' | 'info' | 'success'> = {
  active: 'warning',
  upcoming: 'info',
  ended: 'success'
}

export function MaintenanceStatusBadge({ status }: { status: MaintenanceWindowStatus }) {
  const icon = status === 'active'
    ? <BuildRounded />
    : status === 'upcoming'
      ? <ScheduleRounded />
      : <TaskAltRounded />
  return (
    <Chip
      className={`maintenance-badge maintenance-${status}`}
      icon={icon}
      label={statusLabels[status]}
      color={statusColors[status]}
      size="small"
      variant={status === 'active' ? 'filled' : 'outlined'}
    />
  )
}
