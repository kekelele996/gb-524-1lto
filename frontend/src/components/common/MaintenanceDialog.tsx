import { FormEvent, useEffect, useMemo, useState } from 'react'
import BuildRounded from '@mui/icons-material/BuildRounded'
import { Alert, Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle, Stack, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material'
import { useMaintenanceStore } from '../../stores/maintenanceStore'
import type { MaintenanceWindow } from '../../types/maintenance'
import type { ReceiverStation } from '../../types/station'
import { formatDateTime } from '../../utils/format'

interface MaintenanceDialogProps {
  station: ReceiverStation | null
  canManage: boolean
  onClose: () => void
}

type WindowPhase = 'active' | 'scheduled' | 'finished'

function windowPhase(window: MaintenanceWindow, now: number): WindowPhase {
  const start = new Date(window.start_at).getTime()
  const end = new Date(window.end_at).getTime()
  if (now >= start && now < end) return 'active'
  if (now < start) return 'scheduled'
  return 'finished'
}

function localDateTimeInput(value: string | undefined): string {
  if (!value) return ''
  const date = new Date(value)
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

export function MaintenanceDialog({ station, canManage, onClose }: MaintenanceDialogProps) {
  const windows = useMaintenanceStore((state) => state.windows)
  const load = useMaintenanceStore((state) => state.load)
  const register = useMaintenanceStore((state) => state.register)
  const update = useMaintenanceStore((state) => state.update)
  const busy = useMaintenanceStore((state) => state.busy)
  const [error, setError] = useState<string | null>(null)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [reason, setReason] = useState('')

  useEffect(() => {
    if (station) void load(station.id)
  }, [station, load])

  const now = Date.now()
  const stationWindows = useMemo(
    () => windows.filter((item) => station && item.station_id === station.id),
    [windows, station]
  )

  if (!station) return null

  const resetForm = () => {
    setEditingId(null)
    setStart('')
    setEnd('')
    setReason('')
    setError(null)
  }

  const beginEdit = (window: MaintenanceWindow) => {
    setEditingId(window.id)
    setStart(localDateTimeInput(window.start_at))
    setEnd(localDateTimeInput(window.end_at))
    setReason(window.reason)
    setError(null)
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!start || !end) {
      setError('请填写维护开始与结束时间。')
      return
    }
    const payload = { start_at: new Date(start).toISOString(), end_at: new Date(end).toISOString(), reason: reason.trim() }
    try {
      if (editingId === null) {
        await register({ ...payload, station_id: station.id })
      } else {
        await update(editingId, payload)
      }
      resetForm()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存维护窗口失败')
    }
  }

  return (
    <Dialog open onClose={busy ? undefined : onClose} fullWidth maxWidth="md">
      <DialogTitle>维护窗口 · {station.station_code} · {station.name}</DialogTitle>
      <DialogContent>
        <Stack gap={2} sx={{ pt: 1 }}>
          <Alert severity="warning">窗口生效期间该站不能录入新观测，重跑定位会自动跳过；已有观测和定位结果原样保留。重叠窗口不能保存，窗口结束后站点自动恢复。</Alert>
          <Box className="table-scroll">
            <Table size="small" aria-label="维护窗口列表">
              <TableHead><TableRow><TableCell>开始（本地时间）</TableCell><TableCell>结束（本地时间）</TableCell><TableCell>维护原因</TableCell><TableCell>状态</TableCell>{canManage && <TableCell align="right">操作</TableCell>}</TableRow></TableHead>
              <TableBody>
                {stationWindows.map((window) => {
                  const phase = windowPhase(window, now)
                  const editable = phase !== 'finished'
                  return <TableRow key={window.id} hover>
                    <TableCell>{formatDateTime(window.start_at)}</TableCell>
                    <TableCell>{formatDateTime(window.end_at)}</TableCell>
                    <TableCell>{window.reason}</TableCell>
                    <TableCell>
                      {phase === 'active' && <Chip size="small" color="warning" label="● 生效中" />}
                      {phase === 'scheduled' && <Chip size="small" variant="outlined" label="△ 已排期" />}
                      {phase === 'finished' && <Chip size="small" variant="outlined" label="○ 已结束" />}
                    </TableCell>
                    {canManage && <TableCell align="right">{editable && <Button size="small" startIcon={<BuildRounded />} onClick={() => beginEdit(window)}>修改</Button>}</TableCell>}
                  </TableRow>
                })}
                {stationWindows.length === 0 && <TableRow><TableCell colSpan={canManage ? 5 : 4}>该站尚无维护窗口。</TableCell></TableRow>}
              </TableBody>
            </Table>
          </Box>

          {canManage && (
            <Box component="form" onSubmit={(event) => void submit(event)} className="maintenance-form" aria-label={editingId === null ? '登记维护窗口' : '修改维护窗口'}>
              <Typography component="h3" variant="subtitle2">{editingId === null ? '登记维护窗口' : `修改维护窗口 #${editingId}`}</Typography>
              {error && <Alert severity="error">{error}</Alert>}
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
                <TextField label="开始时间" type="datetime-local" value={start} onChange={(event) => setStart(event.target.value)} InputLabelProps={{ shrink: true }} required fullWidth />
                <TextField label="结束时间" type="datetime-local" value={end} onChange={(event) => setEnd(event.target.value)} InputLabelProps={{ shrink: true }} required fullWidth />
              </Stack>
              <TextField label="维护原因" value={reason} onChange={(event) => setReason(event.target.value)} required inputProps={{ minLength: 2, maxLength: 500 }} helperText="至少 2 个字符，将与登记、修改和定位跳过一起写入审计。" fullWidth />
              <Stack direction="row" justifyContent="flex-end" gap={1}>
                {editingId !== null && <Button onClick={resetForm} disabled={busy}>放弃修改</Button>}
                <Button type="submit" variant="contained" startIcon={<BuildRounded />} disabled={busy}>{busy ? '保存中' : editingId === null ? '登记窗口' : '保存修改'}</Button>
              </Stack>
            </Box>
          )}
        </Stack>
      </DialogContent>
      <DialogActions><Button onClick={onClose} disabled={busy}>关闭</Button></DialogActions>
    </Dialog>
  )
}
