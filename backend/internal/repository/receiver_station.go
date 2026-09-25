package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type StationRepository struct {
	db *gorm.DB
}

func NewStationRepository(db *gorm.DB) *StationRepository {
	return &StationRepository{db: db}
}

func (r *StationRepository) List(ctx context.Context, page, pageSize int, status string) ([]model.ReceiverStation, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := r.db.WithContext(ctx).Model(&model.ReceiverStation{})
	if status != "" {
		query = query.Where("station_status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count receiver stations: %w", err)
	}
	var stations []model.ReceiverStation
	if err := query.Order("station_code ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&stations).Error; err != nil {
		return nil, 0, fmt.Errorf("list receiver stations: %w", err)
	}
	return stations, total, nil
}

func (r *StationRepository) Get(ctx context.Context, id uint) (model.ReceiverStation, error) {
	var station model.ReceiverStation
	if err := r.db.WithContext(ctx).First(&station, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.ReceiverStation{}, api.NewError(404, "STATION_NOT_FOUND", "测向站不存在")
		}
		return model.ReceiverStation{}, fmt.Errorf("get receiver station: %w", err)
	}
	return station, nil
}

func (r *StationRepository) Create(ctx context.Context, station *model.ReceiverStation, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(station).Error; err != nil {
			if err == gorm.ErrDuplicatedKey {
				return api.NewError(409, "STATION_CODE_EXISTS", "测向站编号已存在")
			}
			return fmt.Errorf("create receiver station: %w", err)
		}
		audit := NewAudit(actor, "receiver_station.created", "receiver_station", station.ID, nil, station)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit receiver station create: %w", err)
		}
		return nil
	})
}

func (r *StationRepository) Update(ctx context.Context, station *model.ReceiverStation, before model.ReceiverStation, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ReceiverStation{}).Where("id = ?", station.ID).Updates(map[string]any{
			"name": station.Name, "latitude": station.Latitude, "longitude": station.Longitude,
			"antenna_bias_deg": station.AntennaBiasDeg, "accuracy_deg": station.AccuracyDeg,
			"station_status": station.StationStatus, "calibrated_at": station.CalibratedAt,
		})
		if result.Error != nil {
			return fmt.Errorf("update receiver station: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return api.NewError(404, "STATION_NOT_FOUND", "测向站不存在")
		}
		audit := NewAudit(actor, "receiver_station.calibrated", "receiver_station", station.ID, before, station)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit receiver station update: %w", err)
		}
		return nil
	})
}

func (r *StationRepository) Coverage(ctx context.Context, stationID uint) (int64, *model.BearingObservation, error) {
	query := r.db.WithContext(ctx).Model(&model.BearingObservation{}).Where("station_id = ?", stationID)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, nil, fmt.Errorf("count station observations: %w", err)
	}
	var latest model.BearingObservation
	if err := query.Order("observed_at DESC").First(&latest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return count, nil, nil
		}
		return 0, nil, fmt.Errorf("latest station observation: %w", err)
	}
	return count, &latest, nil
}

// ListMaintenanceWindows 列出维护窗口；stationID 为 0 时返回全部站点。
func (r *StationRepository) ListMaintenanceWindows(ctx context.Context, stationID uint) ([]model.MaintenanceWindow, error) {
	query := r.db.WithContext(ctx).Model(&model.MaintenanceWindow{})
	if stationID > 0 {
		query = query.Where("station_id = ?", stationID)
	}
	var windows []model.MaintenanceWindow
	if err := query.Preload("Station").Order("start_at DESC, id DESC").Find(&windows).Error; err != nil {
		return nil, fmt.Errorf("list maintenance windows: %w", err)
	}
	return windows, nil
}

// GetMaintenanceWindow 返回单个维护窗口。
func (r *StationRepository) GetMaintenanceWindow(ctx context.Context, id uint) (model.MaintenanceWindow, error) {
	var window model.MaintenanceWindow
	if err := r.db.WithContext(ctx).Preload("Station").First(&window, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.MaintenanceWindow{}, api.NewError(404, "MAINTENANCE_WINDOW_NOT_FOUND", "维护窗口不存在")
		}
		return model.MaintenanceWindow{}, fmt.Errorf("get maintenance window: %w", err)
	}
	return window, nil
}

// ActiveMaintenanceAt 返回站点在指定时刻处于生效中的维护窗口；没有则返回 nil。
func (r *StationRepository) ActiveMaintenanceAt(ctx context.Context, stationID uint, at time.Time) (*model.MaintenanceWindow, error) {
	var window model.MaintenanceWindow
	err := r.db.WithContext(ctx).
		Where("station_id = ? AND start_at <= ? AND end_at > ?", stationID, at, at).
		First(&window).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("load active maintenance window: %w", err)
	}
	return &window, nil
}

// ActiveMaintenanceByStationIDs 批量返回给定站点集合在指定时刻处于生效中的窗口。
func (r *StationRepository) ActiveMaintenanceByStationIDs(ctx context.Context, stationIDs []uint, at time.Time) (map[uint]model.MaintenanceWindow, error) {
	result := make(map[uint]model.MaintenanceWindow)
	if len(stationIDs) == 0 {
		return result, nil
	}
	var windows []model.MaintenanceWindow
	if err := r.db.WithContext(ctx).
		Where("station_id IN ? AND start_at <= ? AND end_at > ?", stationIDs, at, at).
		Find(&windows).Error; err != nil {
		return nil, fmt.Errorf("load active maintenance windows: %w", err)
	}
	for _, window := range windows {
		result[window.StationID] = window
	}
	return result, nil
}

// OverlappingMaintenance 返回同一站点与给定时间区间重叠的窗口；excludeID 用于修改时排除自身。
func (r *StationRepository) OverlappingMaintenance(ctx context.Context, stationID uint, start, end time.Time, excludeID uint) ([]model.MaintenanceWindow, error) {
	var windows []model.MaintenanceWindow
	err := r.db.WithContext(ctx).
		Where("station_id = ? AND id <> ? AND start_at < ? AND end_at > ?", stationID, excludeID, end, start).
		Find(&windows).Error
	if err != nil {
		return nil, fmt.Errorf("check overlapping maintenance windows: %w", err)
	}
	return windows, nil
}

func (r *StationRepository) CreateMaintenanceWindow(ctx context.Context, window *model.MaintenanceWindow, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(window).Error; err != nil {
			return fmt.Errorf("create maintenance window: %w", err)
		}
		audit := NewAudit(actor, "maintenance_window.registered", "maintenance_window", window.ID, nil, window)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit maintenance window create: %w", err)
		}
		return nil
	})
}

func (r *StationRepository) UpdateMaintenanceWindow(ctx context.Context, window *model.MaintenanceWindow, before model.MaintenanceWindow, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.MaintenanceWindow{}).Where("id = ?", window.ID).Updates(map[string]any{
			"start_at": window.StartAt, "end_at": window.EndAt, "reason": window.Reason,
		})
		if result.Error != nil {
			return fmt.Errorf("update maintenance window: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return api.NewError(404, "MAINTENANCE_WINDOW_NOT_FOUND", "维护窗口不存在")
		}
		audit := NewAudit(actor, "maintenance_window.modified", "maintenance_window", window.ID, before, window)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit maintenance window update: %w", err)
		}
		return nil
	})
}
