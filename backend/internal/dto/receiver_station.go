package dto

import "time"

type CreateStationRequest struct {
	StationCode    string     `json:"station_code" binding:"required,min=2,max=32"`
	Name           string     `json:"name" binding:"required,min=2,max=120"`
	Latitude       float64    `json:"latitude" binding:"gte=-90,lte=90"`
	Longitude      float64    `json:"longitude" binding:"gte=-180,lte=180"`
	AntennaBiasDeg float64    `json:"antenna_bias_deg" binding:"gte=-30,lte=30"`
	AccuracyDeg    float64    `json:"accuracy_deg" binding:"required,gt=0,lte=45"`
	StationStatus  string     `json:"station_status" binding:"required,oneof=active calibration_due inactive"`
	CalibratedAt   *time.Time `json:"calibrated_at"`
}

type UpdateStationRequest struct {
	Name           string     `json:"name" binding:"required,min=2,max=120"`
	Latitude       float64    `json:"latitude" binding:"gte=-90,lte=90"`
	Longitude      float64    `json:"longitude" binding:"gte=-180,lte=180"`
	AntennaBiasDeg float64    `json:"antenna_bias_deg" binding:"gte=-30,lte=30"`
	AccuracyDeg    float64    `json:"accuracy_deg" binding:"required,gt=0,lte=45"`
	StationStatus  string     `json:"station_status" binding:"required,oneof=active calibration_due inactive"`
	CalibratedAt   *time.Time `json:"calibrated_at"`
}

type StationCoverage struct {
	StationID        uint       `json:"station_id"`
	ObservationCount int64      `json:"observation_count"`
	LastObservedAt   *time.Time `json:"last_observed_at"`
}

type CreateMaintenanceWindowRequest struct {
	StationID uint       `json:"station_id" binding:"required"`
	StartAt   *time.Time `json:"start_at" binding:"required"`
	EndAt     *time.Time `json:"end_at" binding:"required"`
	Reason    string     `json:"reason" binding:"required,min=2,max=500"`
}

type UpdateMaintenanceWindowRequest struct {
	StartAt *time.Time `json:"start_at" binding:"required"`
	EndAt   *time.Time `json:"end_at" binding:"required"`
	Reason  string     `json:"reason" binding:"required,min=2,max=500"`
}

// SkippedStation 记录一次定位运行中被跳过的观测及原因，
// 随定位结果持久化并在定位页展示。
type SkippedStation struct {
	ObservationID       uint       `json:"observation_id"`
	StationID           uint       `json:"station_id"`
	StationCode         string     `json:"station_code"`
	ReasonCode          string     `json:"reason_code"`
	Reason              string     `json:"reason"`
	MaintenanceWindowID *uint      `json:"maintenance_window_id,omitempty"`
	MaintenanceStartAt  *time.Time `json:"maintenance_start_at,omitempty"`
	MaintenanceEndAt    *time.Time `json:"maintenance_end_at,omitempty"`
	MaintenanceReason   string     `json:"maintenance_reason,omitempty"`
}

const (
	SkipReasonMaintenance = "station_in_maintenance"
	SkipReasonInactive    = "station_not_active"
)
