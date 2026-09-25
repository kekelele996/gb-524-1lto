package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

// ---------------------------------------------------------------------------
// 测向站维护窗口（站点聚合内的维护登记，窗口生效时停止录入与参与定位）
// ---------------------------------------------------------------------------

type MaintenanceRepository struct {
	db *gorm.DB
}

func NewMaintenanceRepository(db *gorm.DB) *MaintenanceRepository {
	return &MaintenanceRepository{db: db}
}

// Get 读取单个维护窗口并预载测向站。
func (r *MaintenanceRepository) Get(ctx context.Context, id uint) (model.MaintenanceWindow, error) {
	var window model.MaintenanceWindow
	if err := r.db.WithContext(ctx).Preload("Station").First(&window, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.MaintenanceWindow{}, api.NewError(404, "MAINTENANCE_WINDOW_NOT_FOUND", "维护窗口不存在")
		}
		return model.MaintenanceWindow{}, fmt.Errorf("get maintenance window: %w", err)
	}
	return window, nil
}

// List 按站点（可选）列出维护窗口，未指定站点时可限制数量。
func (r *MaintenanceRepository) List(ctx context.Context, stationID uint, page, pageSize int) ([]model.MaintenanceWindow, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := r.db.WithContext(ctx).Model(&model.MaintenanceWindow{})
	if stationID > 0 {
		query = query.Where("station_id = ?", stationID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count maintenance windows: %w", err)
	}
	var windows []model.MaintenanceWindow
	if err := query.Preload("Station").Order("start_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&windows).Error; err != nil {
		return nil, 0, fmt.Errorf("list maintenance windows: %w", err)
	}
	return windows, total, nil
}

// ActiveForStations 返回给定站点集合在指定时刻生效的维护窗口。
func (r *MaintenanceRepository) ActiveForStations(ctx context.Context, stationIDs []uint, at time.Time) (map[uint]model.MaintenanceWindow, error) {
	result := make(map[uint]model.MaintenanceWindow)
	if len(stationIDs) == 0 {
		return result, nil
	}
	var windows []model.MaintenanceWindow
	if err := r.db.WithContext(ctx).
		Where("station_id IN ?", stationIDs).
		Where("start_at <= ? AND end_at > ?", at.UTC(), at.UTC()).
		Find(&windows).Error; err != nil {
		return nil, fmt.Errorf("load active maintenance windows: %w", err)
	}
	for _, window := range windows {
		// 重叠窗口无法保存，同一站点最多只有一个生效窗口。
		result[window.StationID] = window
	}
	return result, nil
}

// ActiveForStation 返回单个站点在指定时刻生效的维护窗口，无则返回 nil。
func (r *MaintenanceRepository) ActiveForStation(ctx context.Context, stationID uint, at time.Time) (*model.MaintenanceWindow, error) {
	active, err := r.ActiveForStations(ctx, []uint{stationID}, at)
	if err != nil {
		return nil, err
	}
	if window, ok := active[stationID]; ok {
		return &window, nil
	}
	return nil, nil
}

// Create 在事务内串行化同站点登记，重新校验重叠后写入窗口并审计。
func (r *MaintenanceRepository) Create(ctx context.Context, window *model.MaintenanceWindow, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStationRow(tx, window.StationID); err != nil {
			return err
		}
		overlap, err := findOverlap(tx, window.StationID, window.StartAt, window.EndAt, 0)
		if err != nil {
			return err
		}
		if overlap != nil {
			return overlapConflict(*overlap)
		}
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

// Update 在事务内串行化同站点修改，重新校验重叠后更新窗口并审计。
func (r *MaintenanceRepository) Update(ctx context.Context, window *model.MaintenanceWindow, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStationRow(tx, window.StationID); err != nil {
			return err
		}
		var before model.MaintenanceWindow
		if err := tx.First(&before, window.ID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return api.NewError(404, "MAINTENANCE_WINDOW_NOT_FOUND", "维护窗口不存在")
			}
			return fmt.Errorf("load maintenance window for update: %w", err)
		}
		overlap, err := findOverlap(tx, window.StationID, window.StartAt, window.EndAt, window.ID)
		if err != nil {
			return err
		}
		if overlap != nil {
			return overlapConflict(*overlap)
		}
		result := tx.Model(&model.MaintenanceWindow{}).Where("id = ?", window.ID).Updates(map[string]any{
			"start_at": window.StartAt, "end_at": window.EndAt, "reason": window.Reason,
		})
		if result.Error != nil {
			return fmt.Errorf("update maintenance window: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return api.NewError(404, "MAINTENANCE_WINDOW_NOT_FOUND", "维护窗口不存在")
		}
		audit := NewAudit(actor, "maintenance_window.updated", "maintenance_window", window.ID, before, window)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit maintenance window update: %w", err)
		}
		return nil
	})
}

// lockStationRow 串行化同一站点的窗口登记：PostgreSQL 使用行锁，
// SQLite 不支持 SELECT ... FOR UPDATE，改用一次空 UPDATE 获取保留写锁。
func lockStationRow(tx *gorm.DB, stationID uint) error {
	var station model.ReceiverStation
	if err := tx.First(&station, stationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return api.NewError(404, "STATION_NOT_FOUND", "测向站不存在")
		}
		return fmt.Errorf("lock station for maintenance window: %w", err)
	}
	if tx.Dialector.Name() == "sqlite" {
		if err := tx.Exec("UPDATE receiver_stations SET updated_at = updated_at WHERE id = ?", stationID).Error; err != nil {
			return fmt.Errorf("serialize maintenance window: %w", err)
		}
		return nil
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model.ReceiverStation{}, stationID).Error; err != nil {
		return fmt.Errorf("lock station for maintenance window: %w", err)
	}
	return nil
}

// findOverlap 返回与 [startAt, endAt) 重叠的同站点窗口（半开区间，端点相接不算重叠）。
func findOverlap(tx *gorm.DB, stationID uint, startAt, endAt time.Time, excludeID uint) (*model.MaintenanceWindow, error) {
	var window model.MaintenanceWindow
	query := tx.Where("station_id = ?", stationID).
		Where("start_at < ? AND end_at > ?", endAt.UTC(), startAt.UTC())
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	err := query.Order("start_at ASC").First(&window).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check maintenance window overlap: %w", err)
	}
	return &window, nil
}

func overlapConflict(existing model.MaintenanceWindow) error {
	return api.WithDetails(api.NewError(409, "MAINTENANCE_WINDOW_OVERLAP", "该时段与同一测向站已有维护窗口重叠，不能保存"), map[string]any{
		"overlap_window_id": existing.ID,
		"start_at":          existing.StartAt.UTC().Format(time.RFC3339),
		"end_at":            existing.EndAt.UTC().Format(time.RFC3339),
	})
}
