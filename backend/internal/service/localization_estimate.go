package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/datatypes"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/localization"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/internal/repository"
	"spectrum-interference-triangulation/backend/internal/util"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type EstimateService struct {
	repo            *repository.EstimateRepository
	observationRepo *repository.ObservationRepository
	caseRepo        *repository.CaseRepository
	maintenanceRepo *repository.MaintenanceRepository
	conditionLimit  float64
}

type RunResult struct {
	Primary   model.LocalizationEstimate  `json:"primary"`
	Candidate *model.LocalizationEstimate `json:"candidate,omitempty"`
}

func NewEstimateService(repo *repository.EstimateRepository, observationRepo *repository.ObservationRepository, caseRepo *repository.CaseRepository, maintenanceRepo *repository.MaintenanceRepository, conditionLimit float64) *EstimateService {
	return &EstimateService{repo: repo, observationRepo: observationRepo, caseRepo: caseRepo, maintenanceRepo: maintenanceRepo, conditionLimit: conditionLimit}
}

func (s *EstimateService) List(ctx context.Context, caseID uint) ([]model.LocalizationEstimate, error) {
	return s.repo.List(ctx, caseID)
}

func (s *EstimateService) Get(ctx context.Context, id uint) (model.LocalizationEstimate, error) {
	return s.repo.Get(ctx, id)
}

func (s *EstimateService) Run(ctx context.Context, request dto.RunLocalizationRequest, actor repository.Actor) (RunResult, error) {
	if !constants.CanAnalyze(actor.Role) {
		return RunResult{}, api.ErrForbidden
	}
	caseRecord, err := s.caseRepo.Get(ctx, request.CaseID)
	if err != nil {
		return RunResult{}, err
	}
	if caseRecord.CaseStatus != constants.CaseAnalyzing {
		return RunResult{}, api.WithDetails(api.NewError(409, "CASE_NOT_ANALYZING", "只有 analyzing 状态的案例可以运行定位"), map[string]any{"current": caseRecord.CaseStatus})
	}
	observations, err := s.observationRepo.ListForCase(ctx, request.CaseID, false)
	if err != nil {
		return RunResult{}, err
	}
	now := time.Now().UTC()
	stationIDs := make([]uint, 0, len(observations))
	for _, observation := range observations {
		if observation.Station != nil {
			stationIDs = append(stationIDs, observation.StationID)
		}
	}
	activeWindows, err := s.maintenanceRepo.ActiveForStations(ctx, stationIDs, now)
	if err != nil {
		return RunResult{}, err
	}
	inputs := make([]localization.Input, 0, len(observations))
	skipped := make([]dto.SkippedStation, 0)
	for _, observation := range observations {
		if observation.Station == nil {
			continue
		}
		if window, ok := activeWindows[observation.StationID]; ok {
			// 重跑定位跳过维护站：已有观测原样保留，只记录跳过原因。
			windowCopy := window
			skipped = append(skipped, dto.SkippedStation{
				ObservationID: observation.ID, StationID: observation.Station.ID,
				StationCode: observation.Station.StationCode,
				ReasonCode:  dto.SkipReasonMaintenance, Reason: "测向站处于维护窗口，重跑定位时跳过",
				MaintenanceWindowID: &windowCopy.ID,
				MaintenanceStartAt:  &windowCopy.StartAt, MaintenanceEndAt: &windowCopy.EndAt,
				MaintenanceReason: windowCopy.Reason,
			})
			continue
		}
		if observation.Station.StationStatus != "active" {
			skipped = append(skipped, dto.SkippedStation{
				ObservationID: observation.ID, StationID: observation.Station.ID,
				StationCode: observation.Station.StationCode,
				ReasonCode:  dto.SkipReasonInactive, Reason: "测向站未处于启用状态，重跑定位时跳过",
			})
			continue
		}
		if err := validateFrequency(caseRecord.FrequencyCenterHz, observation.FrequencyHz, observation.BandwidthHz); err != nil {
			return RunResult{}, err
		}
		inputs = append(inputs, localization.Input{
			ObservationID: observation.ID, StationCode: observation.Station.StationCode,
			Latitude: observation.Station.Latitude, Longitude: observation.Station.Longitude,
			BearingDeg: observation.CorrectedBearingDeg, AccuracyDeg: observation.Station.AccuracyDeg,
			QualityWeight: constants.QualityWeight(observation.Quality),
		})
	}
	skippedJSON, err := json.Marshal(skipped)
	if err != nil {
		return RunResult{}, fmt.Errorf("marshal skipped station evidence: %w", err)
	}
	run, err := localization.SolveWithOutlierCandidate(inputs, s.conditionLimit, request.AllowOutlier)
	if err != nil {
		var degenerate *localization.DegenerateError
		if errors.As(err, &degenerate) {
			details := map[string]any{
				"condition_number": util.JSONSafeNumber(degenerate.ConditionNumber), "reason": degenerate.Reason,
				"skipped_station_count": len(skipped),
			}
			return RunResult{}, api.WithDetails(api.NewError(422, "GEOMETRY_DEGENERATE", "方位几何退化，无法形成可信定位点"), details)
		}
		if errors.Is(err, localization.ErrInsufficientObservations) {
			return RunResult{}, api.WithDetails(api.NewError(422, "INSUFFICIENT_OBSERVATIONS", "定位至少需要两条来自启用测向站的有效观测"), map[string]any{
				"skipped_station_count": len(skipped),
			})
		}
		return RunResult{}, fmt.Errorf("solve localization: %w", err)
	}
	primary, err := buildEstimate(caseRecord.ID, actor.UserID, constants.EstimateComplete, run.Primary, inputs, skippedJSON)
	if err != nil {
		return RunResult{}, err
	}
	var candidate *model.LocalizationEstimate
	if run.Candidate != nil {
		candidateValue, buildErr := buildEstimate(caseRecord.ID, actor.UserID, constants.EstimateCandidate, *run.Candidate, inputs, skippedJSON)
		if buildErr != nil {
			return RunResult{}, buildErr
		}
		candidate = &candidateValue
	}
	if err := s.repo.CreateRun(ctx, caseRecord.ID, caseRecord.Version, &primary, candidate, skipped, request.AllowOutlier, s.conditionLimit, actor); err != nil {
		return RunResult{}, err
	}
	return RunResult{Primary: primary, Candidate: candidate}, nil
}

func buildEstimate(caseID, userID uint, status string, result localization.Result, inputs []localization.Input, skippedJSON []byte) (model.LocalizationEstimate, error) {
	usedJSON, err := json.Marshal(result.UsedObservationIDs)
	if err != nil {
		return model.LocalizationEstimate{}, fmt.Errorf("marshal used observation IDs: %w", err)
	}
	outlierJSON, err := json.Marshal(result.OutlierIDs)
	if err != nil {
		return model.LocalizationEstimate{}, fmt.Errorf("marshal outlier IDs: %w", err)
	}
	residualJSON, err := json.Marshal(result.Residuals)
	if err != nil {
		return model.LocalizationEstimate{}, fmt.Errorf("marshal residual evidence: %w", err)
	}
	snapshotJSON, err := json.Marshal(inputs)
	if err != nil {
		return model.LocalizationEstimate{}, fmt.Errorf("marshal localization snapshot: %w", err)
	}
	if math.IsNaN(result.Point.Latitude) || math.IsNaN(result.Point.Longitude) {
		return model.LocalizationEstimate{}, api.NewError(422, "INVALID_ESTIMATE", "定位算法产生了无效坐标")
	}
	return model.LocalizationEstimate{
		CaseID: caseID, AlgorithmVersion: localization.AlgorithmVersion,
		Latitude: result.Point.Latitude, Longitude: result.Point.Longitude,
		UncertaintyRadiusM: result.UncertaintyRadiusM, ResidualDeg: result.ResidualDeg,
		ConditionNumber: result.ConditionNumber, GeometryDegenerate: false,
		UsedObservationIDsJSON: datatypes.JSON(usedJSON), OutlierIDsJSON: datatypes.JSON(outlierJSON),
		ResidualsJSON: datatypes.JSON(residualJSON), InputSnapshotJSON: datatypes.JSON(snapshotJSON),
		SkippedStationsJSON: datatypes.JSON(skippedJSON),
		EstimateStatus:      status, CreatedBy: userID,
	}, nil
}
