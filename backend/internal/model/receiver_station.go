package model

import "time"

type ReceiverStation struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	StationCode    string     `json:"station_code" gorm:"size:32;not null;uniqueIndex"`
	Name           string     `json:"name" gorm:"size:120;not null"`
	Latitude       float64    `json:"latitude" gorm:"not null;check:latitude >= -90 AND latitude <= 90"`
	Longitude      float64    `json:"longitude" gorm:"not null;check:longitude >= -180 AND longitude <= 180"`
	AntennaBiasDeg float64    `json:"antenna_bias_deg" gorm:"not null;default:0;check:antenna_bias_deg >= -30 AND antenna_bias_deg <= 30"`
	AccuracyDeg    float64    `json:"accuracy_deg" gorm:"not null;check:accuracy_deg > 0 AND accuracy_deg <= 45"`
	StationStatus  string     `json:"station_status" gorm:"size:16;not null;default:active;check:station_status IN ('active','calibration_due','inactive')"`
	CalibratedAt   *time.Time `json:"calibrated_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (ReceiverStation) TableName() string { return "receiver_stations" }

// MaintenanceWindow 记录测向站的计划维护时段。窗口生效期间站点暂停录入新观测、
// 不参与新定位；窗口到期后按当前时刻自动恢复，无需变更站点状态。
// 它是 ReceiverStation 的子实体，因此与站点同层维护。
type MaintenanceWindow struct {
	ID        uint             `json:"id" gorm:"primaryKey"`
	StationID uint             `json:"station_id" gorm:"not null;index:idx_maintenance_station_time,priority:1"`
	StartAt   time.Time        `json:"start_at" gorm:"not null;index:idx_maintenance_station_time,priority:2;check:chk_maintenance_window_order,end_at > start_at"`
	EndAt     time.Time        `json:"end_at" gorm:"not null;index:idx_maintenance_station_time,priority:3"`
	Reason    string           `json:"reason" gorm:"size:500;not null"`
	CreatedBy uint             `json:"created_by" gorm:"not null"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Station   *ReceiverStation `json:"station,omitempty" gorm:"foreignKey:StationID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (MaintenanceWindow) TableName() string { return "maintenance_windows" }

// MaintenanceWindowStatus 是按当前时刻推导的逻辑状态，不入库。
type MaintenanceWindowStatus string

const (
	MaintenanceUpcoming MaintenanceWindowStatus = "upcoming"
	MaintenanceActive   MaintenanceWindowStatus = "active"
	MaintenanceEnded    MaintenanceWindowStatus = "ended"
)

// StatusAt 按给定当前时刻推导窗口状态。
func (w MaintenanceWindow) StatusAt(now time.Time) MaintenanceWindowStatus {
	if now.Before(w.StartAt) {
		return MaintenanceUpcoming
	}
	if !now.Before(w.EndAt) {
		return MaintenanceEnded
	}
	return MaintenanceActive
}

// MaintenanceWindowsOverlap 判断同一站点两个维护窗口是否在时间轴上重叠。
// 端点相接（前一个 end 等于后一个 start）允许保存，便于连续维护分段登记。
func MaintenanceWindowsOverlap(startA, endA, startB, endB time.Time) bool {
	return startA.Before(endB) && startB.Before(endA)
}
