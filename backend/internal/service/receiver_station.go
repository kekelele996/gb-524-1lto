package service

import (
	"context"
	"strings"
	"time"

	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/internal/repository"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type StationService struct {
	repo *repository.StationRepository
}

func NewStationService(repo *repository.StationRepository) *StationService {
	return &StationService{repo: repo}
}

// minMaintenanceDuration 防止录入无意义的零长度窗口。
const minMaintenanceDuration = time.Minute

func (s *StationService) ListMaintenanceWindows(ctx context.Context, stationID uint) ([]dto.MaintenanceWindowResponse, error) {
	if stationID > 0 {
		if _, err := s.repo.Get(ctx, stationID); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	windows, err := s.repo.ListMaintenanceWindows(ctx, stationID)
	if err != nil {
		return nil, err
	}
	return decorateMaintenanceWindows(windows, now), nil
}

func (s *StationService) CreateMaintenanceWindow(ctx context.Context, stationID uint, request dto.CreateMaintenanceWindowRequest, actor repository.Actor) (dto.MaintenanceWindowResponse, error) {
	station, err := s.repo.Get(ctx, stationID)
	if err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	now := time.Now().UTC()
	start := request.StartAt.UTC()
	end := request.EndAt.UTC()
	if err := validateMaintenanceWindowRange(start, end, now); err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	if err := s.ensureNoMaintenanceOverlap(ctx, stationID, start, end, 0); err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	window := model.MaintenanceWindow{
		StationID: station.ID, StartAt: start, EndAt: end,
		Reason: strings.TrimSpace(request.Reason), CreatedBy: actor.UserID,
	}
	if err := s.repo.CreateMaintenanceWindow(ctx, &window, actor); err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	window.Station = &station
	return dto.MaintenanceWindowResponse{MaintenanceWindow: window, Status: window.StatusAt(now)}, nil
}

func (s *StationService) UpdateMaintenanceWindow(ctx context.Context, id uint, request dto.UpdateMaintenanceWindowRequest, actor repository.Actor) (dto.MaintenanceWindowResponse, error) {
	before, err := s.repo.GetMaintenanceWindow(ctx, id)
	if err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	now := time.Now().UTC()
	currentStatus := before.StatusAt(now)
	if currentStatus == model.MaintenanceEnded {
		return dto.MaintenanceWindowResponse{}, api.NewError(409, "MAINTENANCE_WINDOW_LOCKED", "已结束的维护窗口作为历史证据不可修改")
	}
	start := request.StartAt.UTC()
	end := request.EndAt.UTC()
	if err := validateMaintenanceWindowRange(start, end, now); err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	if currentStatus == model.MaintenanceActive {
		// 窗口生效中只能延长或缩短结束时间，开始时间保持不变，避免改写已生效证据。
		if !start.Equal(before.StartAt) {
			return dto.MaintenanceWindowResponse{}, api.NewError(422, "MAINTENANCE_START_LOCKED", "窗口生效后不能调整开始时间，只能修改结束时间与原因")
		}
		if !end.After(now) {
			return dto.MaintenanceWindowResponse{}, api.NewError(422, "MAINTENANCE_END_REQUIRED", "生效中的窗口新的结束时间必须晚于当前时间")
		}
	}
	if err := s.ensureNoMaintenanceOverlap(ctx, before.StationID, start, end, id); err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	updated := before
	updated.StartAt = start
	updated.EndAt = end
	updated.Reason = strings.TrimSpace(request.Reason)
	if err := s.repo.UpdateMaintenanceWindow(ctx, &updated, before, actor); err != nil {
		return dto.MaintenanceWindowResponse{}, err
	}
	return dto.MaintenanceWindowResponse{MaintenanceWindow: updated, Status: updated.StatusAt(now)}, nil
}

func (s *StationService) ensureNoMaintenanceOverlap(ctx context.Context, stationID uint, start, end time.Time, excludeID uint) error {
	overlapping, err := s.repo.OverlappingMaintenance(ctx, stationID, start, end, excludeID)
	if err != nil {
		return err
	}
	if len(overlapping) > 0 {
		conflict := overlapping[0]
		return api.WithDetails(api.NewError(409, "MAINTENANCE_WINDOW_OVERLAP", "同一测向站的维护窗口时间重叠，不能保存"), map[string]any{
			"conflict_window_id": conflict.ID,
			"conflict_start_at":  conflict.StartAt,
			"conflict_end_at":    conflict.EndAt,
		})
	}
	return nil
}

func validateMaintenanceWindowRange(start, end, now time.Time) error {
	if !end.After(start) {
		return api.NewError(422, "INVALID_MAINTENANCE_RANGE", "维护结束时间必须晚于开始时间")
	}
	if end.Sub(start) < minMaintenanceDuration {
		return api.NewError(422, "INVALID_MAINTENANCE_RANGE", "维护窗口时长至少为 1 分钟")
	}
	if !end.After(now) {
		return api.NewError(422, "MAINTENANCE_WINDOW_IN_PAST", "维护结束时间必须晚于当前时间，不能登记已经完全结束的窗口")
	}
	return nil
}

func decorateMaintenanceWindows(windows []model.MaintenanceWindow, now time.Time) []dto.MaintenanceWindowResponse {
	result := make([]dto.MaintenanceWindowResponse, 0, len(windows))
	for _, window := range windows {
		result = append(result, dto.MaintenanceWindowResponse{
			MaintenanceWindow: window,
			Status:            window.StatusAt(now),
		})
	}
	return result
}

func (s *StationService) List(ctx context.Context, page, pageSize int, status string) ([]model.ReceiverStation, int64, error) {
	if status != "" && status != "active" && status != "calibration_due" && status != "inactive" {
		return nil, 0, api.NewError(400, "INVALID_STATION_STATUS", "测向站状态筛选值无效")
	}
	return s.repo.List(ctx, page, pageSize, status)
}

func (s *StationService) Get(ctx context.Context, id uint) (model.ReceiverStation, error) {
	return s.repo.Get(ctx, id)
}

func (s *StationService) Create(ctx context.Context, request dto.CreateStationRequest, actor repository.Actor) (model.ReceiverStation, error) {
	if err := validateCoordinates(request.Latitude, request.Longitude); err != nil {
		return model.ReceiverStation{}, err
	}
	station := model.ReceiverStation{
		StationCode:    strings.ToUpper(strings.TrimSpace(request.StationCode)),
		Name:           strings.TrimSpace(request.Name),
		Latitude:       request.Latitude,
		Longitude:      request.Longitude,
		AntennaBiasDeg: request.AntennaBiasDeg,
		AccuracyDeg:    request.AccuracyDeg,
		StationStatus:  request.StationStatus,
		CalibratedAt:   request.CalibratedAt,
	}
	if station.StationStatus == "active" && station.CalibratedAt == nil {
		return model.ReceiverStation{}, api.NewError(422, "CALIBRATION_REQUIRED", "启用测向站前必须填写最近校准时间")
	}
	if err := s.repo.Create(ctx, &station, actor); err != nil {
		return model.ReceiverStation{}, err
	}
	return station, nil
}

func (s *StationService) Update(ctx context.Context, id uint, request dto.UpdateStationRequest, actor repository.Actor) (model.ReceiverStation, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		return model.ReceiverStation{}, err
	}
	if err := validateCoordinates(request.Latitude, request.Longitude); err != nil {
		return model.ReceiverStation{}, err
	}
	if request.CalibratedAt != nil && request.CalibratedAt.After(time.Now().UTC().Add(5*time.Minute)) {
		return model.ReceiverStation{}, api.NewError(422, "INVALID_CALIBRATION_TIME", "校准时间不能晚于当前时间")
	}
	updated := before
	updated.Name = strings.TrimSpace(request.Name)
	updated.Latitude = request.Latitude
	updated.Longitude = request.Longitude
	updated.AntennaBiasDeg = request.AntennaBiasDeg
	updated.AccuracyDeg = request.AccuracyDeg
	updated.StationStatus = request.StationStatus
	updated.CalibratedAt = request.CalibratedAt
	if updated.StationStatus == "active" && updated.CalibratedAt == nil {
		return model.ReceiverStation{}, api.NewError(422, "CALIBRATION_REQUIRED", "启用测向站前必须填写最近校准时间")
	}
	if err := s.repo.Update(ctx, &updated, before, actor); err != nil {
		return model.ReceiverStation{}, err
	}
	return updated, nil
}

func (s *StationService) Coverage(ctx context.Context, id uint) (dto.StationCoverage, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return dto.StationCoverage{}, err
	}
	count, latest, err := s.repo.Coverage(ctx, id)
	if err != nil {
		return dto.StationCoverage{}, err
	}
	coverage := dto.StationCoverage{StationID: id, ObservationCount: count}
	if latest != nil {
		coverage.LastObservedAt = &latest.ObservedAt
	}
	return coverage, nil
}

func validateCoordinates(latitude, longitude float64) error {
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return api.WithDetails(api.NewError(422, "INVALID_COORDINATES", "经纬度超出 WGS84 有效范围"), map[string]any{
			"latitude": latitude, "longitude": longitude,
		})
	}
	return nil
}
