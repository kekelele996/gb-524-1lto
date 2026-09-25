import { FormEvent, useEffect, useState } from 'react'
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography } from '@mui/material'
import type { MaintenanceWindow, MaintenanceWindowInput } from '../../types/maintenance'

interface MaintenanceDialogProps {
  open: boolean
  stationName: string
  window: MaintenanceWindow | null
  saving: boolean
  onClose: () => void
  onSubmit: (input: MaintenanceWindowInput) => Promise<void>
}

function toLocalInputValue(iso: string): string {
  // datetime-local 使用不带时区的本地时间，取出前 16 位 yyyy-MM-ddThh:mm。
  const date = new Date(iso)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function MaintenanceDialog({ open, stationName, window, saving, onClose, onSubmit }: MaintenanceDialogProps) {
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [reason, setReason] = useState('')

  useEffect(() => {
    if (!open) return
    if (window) {
      setStart(toLocalInputValue(window.start_at))
      setEnd(toLocalInputValue(window.end_at))
      setReason(window.reason)
    } else {
      const now = new Date()
      setStart(toLocalInputValue(now.toISOString()))
      setEnd(toLocalInputValue(new Date(now.getTime() + 60 * 60 * 1000).toISOString()))
      setReason('')
    }
  }, [open, window])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!start || !end) return
    await onSubmit({
      start_at: new Date(start).toISOString(),
      end_at: new Date(end).toISOString(),
      reason: reason.trim()
    })
  }

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} fullWidth maxWidth="sm">
      <form onSubmit={(event) => void submit(event)}>
        <DialogTitle>{window ? `修改维护窗口 #${window.id}` : `登记维护窗口`} · {stationName}</DialogTitle>
        <DialogContent>
          <Stack gap={2} sx={{ pt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              窗口生效期间该站不能录入新观测、不参与新定位；已有观测与定位结果原样保留，窗口结束后自动恢复。
            </Typography>
            <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
              <TextField
                label="开始时间" type="datetime-local" value={start} onChange={(event) => setStart(event.target.value)}
                required fullWidth InputLabelProps={{ shrink: true }}
                disabled={window?.status === 'active'}
                helperText={window?.status === 'active' ? '窗口已生效，开始时间锁定' : ' '}
              />
              <TextField
                label="结束时间" type="datetime-local" value={end} onChange={(event) => setEnd(event.target.value)}
                required fullWidth InputLabelProps={{ shrink: true }} helperText="结束后站点自动恢复使用"
              />
            </Stack>
            <TextField
              label="维护原因" value={reason} onChange={(event) => setReason(event.target.value)}
              required multiline minRows={3} inputProps={{ minLength: 4, maxLength: 500 }}
              helperText="至少 4 个字符；登记与修改均写入不可变审计。"
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose} disabled={saving}>继续查看</Button>
          <Button type="submit" variant="contained" color="warning" disabled={saving || reason.trim().length < 4 || !start || !end}>
            {window ? '保存修改' : '登记维护窗口'}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  )
}
