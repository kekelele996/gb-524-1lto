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

// MaintenanceWindow 登记测向站暂停服务的时间区间。窗口生效期间站点不能录入新观测、
// 不参与新定位；区间结束后站点按 station_status 自动恢复使用。
type MaintenanceWindow struct {
	ID        uint             `json:"id" gorm:"primaryKey"`
	StationID uint             `json:"station_id" gorm:"not null;index:idx_maintenance_station,priority:1"`
	StartAt   time.Time        `json:"start_at" gorm:"not null;index:idx_maintenance_station,priority:2;index"`
	EndAt     time.Time        `json:"end_at" gorm:"not null;index;check:end_at > start_at"`
	Reason    string           `json:"reason" gorm:"size:500;not null"`
	CreatedBy uint             `json:"created_by" gorm:"not null"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Station   *ReceiverStation `json:"station,omitempty" gorm:"foreignKey:StationID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (MaintenanceWindow) TableName() string { return "maintenance_windows" }

// ActiveAt 判断窗口在指定时刻是否生效：左闭右开区间，结束时刻自动恢复。
func (w MaintenanceWindow) ActiveAt(at time.Time) bool {
	at = at.UTC()
	return !at.Before(w.StartAt.UTC()) && at.Before(w.EndAt.UTC())
}
