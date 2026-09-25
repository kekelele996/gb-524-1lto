import { FormEvent, useEffect, useMemo, useState } from 'react'
import AddRounded from '@mui/icons-material/AddRounded'
import CalibrationRounded from '@mui/icons-material/CompassCalibrationRounded'
import BuildRounded from '@mui/icons-material/BuildRounded'
import { Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle, MenuItem, Stack, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material'
import { BearingPlot } from '../components/common/BearingPlot'
import { MaintenanceDialog } from '../components/common/MaintenanceDialog'
import { PageHeader } from '../components/common/PageHeader'
import { useAuth } from '../hooks/useAuth'
import { useMaintenanceStore } from '../stores/maintenanceStore'
import { useObservationStore } from '../stores/observationStore'
import { useStationStore } from '../stores/stationStore'
import type { ReceiverStation, StationInput } from '../types/station'
import { formatCoordinate, formatDateTime, formatDecimal } from '../utils/format'

const initialStation: StationInput = {
  station_code: '', name: '', latitude: 31.2304, longitude: 121.4737,
  antenna_bias_deg: 0, accuracy_deg: 1.5, station_status: 'active', calibrated_at: new Date().toISOString()
}

export function StationsPage() {
  const { hasRole } = useAuth()
  const stations = useStationStore((state) => state.stations)
  const loadStations = useStationStore((state) => state.load)
  const createStation = useStationStore((state) => state.createStation)
  const busy = useStationStore((state) => state.busy)
  const observations = useObservationStore((state) => state.observations)
  const loadObservations = useObservationStore((state) => state.load)
  const windows = useMaintenanceStore((state) => state.windows)
  const loadWindows = useMaintenanceStore((state) => state.load)
  const [open, setOpen] = useState(false)
  const [maintenanceStation, setMaintenanceStation] = useState<ReceiverStation | null>(null)
  const [form, setForm] = useState<StationInput>(initialStation)
  const [saving, setSaving] = useState(false)
  const canManage = hasRole('analyst', 'admin')

  useEffect(() => {
    void Promise.all([loadStations(), loadObservations(), loadWindows()])
  }, [loadStations, loadObservations, loadWindows])

  const activeWindowByStation = useMemo(() => {
    const map = new Map<number, (typeof windows)[number]>()
    const now = Date.now()
    for (const window of windows) {
      const start = new Date(window.start_at).getTime()
      const end = new Date(window.end_at).getTime()
      if (now >= start && now < end) map.set(window.station_id, window)
    }
    return map
  }, [windows])

  const usableCount = useMemo(
    () => stations.filter((station) => station.station_status === 'active' && !activeWindowByStation.has(station.id)).length,
    [stations, activeWindowByStation]
  )

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    try {
      await createStation(form)
      setOpen(false)
      setForm({ ...initialStation, calibrated_at: new Date().toISOString() })
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <PageHeader
        eyebrow="RECEIVER GEOMETRY / CALIBRATION"
        title="测向站与观测覆盖"
        summary={`${stations.length} 个测向站 · ${usableCount} 个当前可用 · 坐标仅用于离线局部平面计算`}
        actions={canManage ? <Button variant="contained" startIcon={<AddRounded />} onClick={() => setOpen(true)}>登记测向站</Button> : undefined}
      />

      <section className="plot-section">
        <BearingPlot stations={stations} observations={observations.slice(0, 20)} height={390} />
      </section>

      <section className="data-section" aria-labelledby="station-table-title">
        <Stack direction="row" justifyContent="space-between" alignItems="baseline" mb={2}>
          <Typography id="station-table-title" component="h2" variant="h6">站点校准台账</Typography>
          <Typography variant="body2" color="text.secondary">天线偏置在保存观测时写入校正方位；维护窗口期间不录入、不参与定位</Typography>
        </Stack>
        <Box className="table-scroll">
          <Table size="small" aria-label="测向站列表">
            <TableHead><TableRow><TableCell>站点</TableCell><TableCell>WGS84 坐标</TableCell><TableCell>精度 / 偏置</TableCell><TableCell>状态</TableCell><TableCell>最近校准</TableCell><TableCell align="right">维护窗口</TableCell></TableRow></TableHead>
            <TableBody>
              {stations.map((station) => {
                const activeWindow = activeWindowByStation.get(station.id)
                return (
                  <TableRow key={station.id} hover className={activeWindow ? 'row-warning' : ''}>
                    <TableCell><strong>{station.station_code}</strong><br /><span className="secondary-text">{station.name}</span></TableCell>
                    <TableCell className="numeric">{formatCoordinate(station.latitude)}<br />{formatCoordinate(station.longitude)}</TableCell>
                    <TableCell className="numeric">±{formatDecimal(station.accuracy_deg, 1)}° / {station.antenna_bias_deg >= 0 ? '+' : ''}{formatDecimal(station.antenna_bias_deg, 1)}°</TableCell>
                    <TableCell>
                      <span className={`status-text status-${station.station_status}`}>{station.station_status === 'active' ? '● 已启用' : station.station_status === 'calibration_due' ? '△ 待校准' : '○ 已停用'}</span>
                      {activeWindow && <><br /><Chip size="small" color="warning" label="● 维护中" sx={{ mt: 0.5 }} /></>}
                    </TableCell>
                    <TableCell>{formatDateTime(station.calibrated_at)}</TableCell>
                    <TableCell align="right">
                      <Button size="small" startIcon={<BuildRounded />} color={activeWindow ? 'warning' : 'inherit'} onClick={() => setMaintenanceStation(station)}>
                        {activeWindow ? `至 ${formatDateTime(activeWindow.end_at)}` : '登记 / 查看'}
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
              {!busy && stations.length === 0 && <TableRow><TableCell colSpan={6}>尚无测向站。登记并校准站点后才能录入方位观测。</TableCell></TableRow>}
            </TableBody>
          </Table>
        </Box>
      </section>

      <Dialog open={open} onClose={saving ? undefined : () => setOpen(false)} fullWidth maxWidth="sm">
        <form onSubmit={(event) => void submit(event)}>
          <DialogTitle>登记离线测向站</DialogTitle>
          <DialogContent>
            <Stack gap={2} sx={{ pt: 1 }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
                <TextField label="站点编号" value={form.station_code} onChange={(event) => setForm({ ...form, station_code: event.target.value })} required fullWidth />
                <TextField label="站点名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required fullWidth />
              </Stack>
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
                <TextField label="纬度" type="number" inputProps={{ step: '0.000001' }} value={form.latitude} onChange={(event) => setForm({ ...form, latitude: Number(event.target.value) })} required fullWidth />
                <TextField label="经度" type="number" inputProps={{ step: '0.000001' }} value={form.longitude} onChange={(event) => setForm({ ...form, longitude: Number(event.target.value) })} required fullWidth />
              </Stack>
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
                <TextField label="精度（度）" type="number" inputProps={{ step: '0.1', min: 0.1, max: 45 }} value={form.accuracy_deg} onChange={(event) => setForm({ ...form, accuracy_deg: Number(event.target.value) })} required fullWidth />
                <TextField label="天线偏置（度）" type="number" inputProps={{ step: '0.1', min: -30, max: 30 }} value={form.antenna_bias_deg} onChange={(event) => setForm({ ...form, antenna_bias_deg: Number(event.target.value) })} fullWidth />
              </Stack>
              <TextField select label="站点状态" value={form.station_status} onChange={(event) => setForm({ ...form, station_status: event.target.value as StationInput['station_status'] })}>
                <MenuItem value="active">已启用</MenuItem><MenuItem value="calibration_due">待校准</MenuItem><MenuItem value="inactive">已停用</MenuItem>
              </TextField>
              <TextField label="校准时间" type="datetime-local" value={form.calibrated_at?.slice(0, 16) ?? ''} onChange={(event) => setForm({ ...form, calibrated_at: event.target.value ? new Date(event.target.value).toISOString() : null })} InputLabelProps={{ shrink: true }} />
            </Stack>
          </DialogContent>
          <DialogActions><Button onClick={() => setOpen(false)} disabled={saving}>继续查看</Button><Button type="submit" variant="contained" startIcon={<CalibrationRounded />} disabled={saving}>保存校准站点</Button></DialogActions>
        </form>
      </Dialog>

      <MaintenanceDialog station={maintenanceStation} canManage={canManage} onClose={() => setMaintenanceStation(null)} />
    </>
  )
}

