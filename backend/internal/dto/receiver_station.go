package dto

import (
	"time"

	"spectrum-interference-triangulation/backend/internal/model"
)

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

// 维护窗口是测向站的子实体，与站点 DTO 同文件维护。

type CreateMaintenanceWindowRequest struct {
	StartAt time.Time `json:"start_at" binding:"required"`
	EndAt   time.Time `json:"end_at" binding:"required"`
	Reason  string    `json:"reason" binding:"required,min=4,max=500"`
}

type UpdateMaintenanceWindowRequest struct {
	StartAt time.Time `json:"start_at" binding:"required"`
	EndAt   time.Time `json:"end_at" binding:"required"`
	Reason  string    `json:"reason" binding:"required,min=4,max=500"`
}

// MaintenanceWindowResponse 附带按当前时刻推导的逻辑状态，前端无需重复计算。
type MaintenanceWindowResponse struct {
	model.MaintenanceWindow
	Status model.MaintenanceWindowStatus `json:"status"`
}

// SkippedObservation 说明一次定位运行中因维护窗口而跳过的观测及原因。
type SkippedObservation struct {
	ObservationID uint   `json:"observation_id"`
	StationID     uint   `json:"station_id"`
	StationCode   string `json:"station_code"`
	Reason        string `json:"reason"`
	WindowEndsAt  string `json:"window_ends_at"`
}
