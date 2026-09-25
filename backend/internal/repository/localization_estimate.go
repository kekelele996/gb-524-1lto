package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type EstimateRepository struct {
	db *gorm.DB
}

func NewEstimateRepository(db *gorm.DB) *EstimateRepository {
	return &EstimateRepository{db: db}
}

func (r *EstimateRepository) List(ctx context.Context, caseID uint) ([]model.LocalizationEstimate, error) {
	query := r.db.WithContext(ctx).Model(&model.LocalizationEstimate{})
	if caseID > 0 {
		query = query.Where("case_id = ?", caseID)
	}
	var estimates []model.LocalizationEstimate
	if err := query.Order("created_at DESC, id DESC").Find(&estimates).Error; err != nil {
		return nil, fmt.Errorf("list localization estimates: %w", err)
	}
	return estimates, nil
}

func (r *EstimateRepository) Get(ctx context.Context, id uint) (model.LocalizationEstimate, error) {
	var estimate model.LocalizationEstimate
	if err := r.db.WithContext(ctx).First(&estimate, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.LocalizationEstimate{}, api.NewError(404, "ESTIMATE_NOT_FOUND", "定位结果不存在")
		}
		return model.LocalizationEstimate{}, fmt.Errorf("get localization estimate: %w", err)
	}
	return estimate, nil
}

func (r *EstimateRepository) CreateRun(ctx context.Context, caseID, version uint, primary *model.LocalizationEstimate, candidate *model.LocalizationEstimate, skipped []dto.SkippedStation, allowOutlier bool, conditionLimit float64, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&model.InterferenceCase{}).
			Where("id = ? AND version = ? AND case_status = ?", caseID, version, constants.CaseAnalyzing).
			UpdateColumn("version", gorm.Expr("version + 1"))
		if claim.Error != nil {
			return fmt.Errorf("claim case localization run: %w", claim.Error)
		}
		if claim.RowsAffected != 1 {
			return api.NewError(409, "CASE_VERSION_CONFLICT", "案例状态或版本已变化，请刷新后重试")
		}
		if err := tx.Create(primary).Error; err != nil {
			return fmt.Errorf("save primary estimate: %w", err)
		}
		if candidate != nil {
			candidate.ParentEstimateID = &primary.ID
			if err := tx.Create(candidate).Error; err != nil {
				return fmt.Errorf("save outlier candidate estimate: %w", err)
			}
		}
		after := map[string]any{
			"primary_id":             primary.ID,
			"algorithm_version":      primary.AlgorithmVersion,
			"allow_outlier":          allowOutlier,
			"condition_limit":        conditionLimit,
			"candidate_id":           candidateID(candidate),
			"residual_deg":           primary.ResidualDeg,
			"condition_number":       primary.ConditionNumber,
			"geometry_degenerate":    primary.GeometryDegenerate,
			"used_observation_ids":   primary.UsedObservationIDsJSON,
			"outlier_ids":            primary.OutlierIDsJSON,
			"skipped_station_count":  len(skipped),
			"skipped_stations":       skipped,
			"maintenance_skip_count": countMaintenanceSkips(skipped),
		}
		audit := NewAudit(actor, "localization_estimate.created", "interference_case", caseID, map[string]any{"version": version}, after)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit localization run: %w", err)
		}
		// 每次重跑因维护窗口跳过站点都单独写入审计，绑定窗口实体以便复核追溯。
		for _, skip := range skipped {
			if skip.ReasonCode != dto.SkipReasonMaintenance || skip.MaintenanceWindowID == nil {
				continue
			}
			skipAudit := NewAudit(actor, "maintenance_window.localization_skipped", "maintenance_window", *skip.MaintenanceWindowID, nil, map[string]any{
				"case_id": caseID, "estimate_id": primary.ID, "observation_id": skip.ObservationID,
				"station_id": skip.StationID, "station_code": skip.StationCode,
				"window_start_at": skip.MaintenanceStartAt, "window_end_at": skip.MaintenanceEndAt,
				"window_reason": skip.MaintenanceReason, "skip_reason": skip.Reason,
			})
			if err := tx.Create(&skipAudit).Error; err != nil {
				return fmt.Errorf("audit maintenance skip: %w", err)
			}
		}
		return nil
	})
}

func candidateID(candidate *model.LocalizationEstimate) uint {
	if candidate == nil {
		return 0
	}
	return candidate.ID
}

func countMaintenanceSkips(skipped []dto.SkippedStation) int {
	count := 0
	for _, skip := range skipped {
		if skip.ReasonCode == dto.SkipReasonMaintenance {
			count++
		}
	}
	return count
}
