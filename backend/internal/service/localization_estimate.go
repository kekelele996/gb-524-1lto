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
	stationRepo     *repository.StationRepository
	conditionLimit  float64
}

type RunResult struct {
	Primary   model.LocalizationEstimate  `json:"primary"`
	Candidate *model.LocalizationEstimate `json:"candidate,omitempty"`
	Skipped   []dto.SkippedObservation    `json:"skipped"`
}

func NewEstimateService(repo *repository.EstimateRepository, observationRepo *repository.ObservationRepository, caseRepo *repository.CaseRepository, stationRepo *repository.StationRepository, conditionLimit float64) *EstimateService {
	return &EstimateService{repo: repo, observationRepo: observationRepo, caseRepo: caseRepo, stationRepo: stationRepo, conditionLimit: conditionLimit}
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
	stationIDs := make([]uint, 0, len(observations))
	for _, observation := range observations {
		if observation.Station != nil {
			stationIDs = append(stationIDs, observation.Station.ID)
		}
	}
	maintenanceByStation, err := s.stationRepo.ActiveMaintenanceByStationIDs(ctx, stationIDs, time.Now().UTC())
	if err != nil {
		return RunResult{}, err
	}
	inputs := make([]localization.Input, 0, len(observations))
	skipped := make([]dto.SkippedObservation, 0)
	for _, observation := range observations {
		if observation.Station == nil || observation.Station.StationStatus != "active" {
			continue
		}
		if window, underMaintenance := maintenanceByStation[observation.Station.ID]; underMaintenance {
			skipped = append(skipped, dto.SkippedObservation{
				ObservationID: observation.ID,
				StationID:     observation.Station.ID,
				StationCode:   observation.Station.StationCode,
				Reason:        "测向站处于维护窗口，暂停参与定位，窗口结束后自动恢复",
				WindowEndsAt:  window.EndAt.Format(time.RFC3339),
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
	run, err := localization.SolveWithOutlierCandidate(inputs, s.conditionLimit, request.AllowOutlier)
	if err != nil {
		var degenerate *localization.DegenerateError
		if errors.As(err, &degenerate) {
			return RunResult{}, api.WithDetails(api.NewError(422, "GEOMETRY_DEGENERATE", "方位几何退化，无法形成可信定位点"), map[string]any{
				"condition_number": util.JSONSafeNumber(degenerate.ConditionNumber), "reason": degenerate.Reason,
			})
		}
		if errors.Is(err, localization.ErrInsufficientObservations) {
			appErr := api.NewError(422, "INSUFFICIENT_OBSERVATIONS", "定位至少需要两条来自未维护、启用测向站的有效观测")
			if len(skipped) > 0 {
				appErr = api.WithDetails(appErr, map[string]any{"skipped": skipped})
			}
			return RunResult{}, appErr
		}
		return RunResult{}, fmt.Errorf("solve localization: %w", err)
	}
	primary, err := buildEstimate(caseRecord.ID, actor.UserID, constants.EstimateComplete, run.Primary, inputs, skipped)
	if err != nil {
		return RunResult{}, err
	}
	var candidate *model.LocalizationEstimate
	if run.Candidate != nil {
		candidateValue, buildErr := buildEstimate(caseRecord.ID, actor.UserID, constants.EstimateCandidate, *run.Candidate, inputs, skipped)
		if buildErr != nil {
			return RunResult{}, buildErr
		}
		candidate = &candidateValue
	}
	if err := s.repo.CreateRun(ctx, caseRecord.ID, caseRecord.Version, &primary, candidate, request.AllowOutlier, s.conditionLimit, skipped, actor); err != nil {
		return RunResult{}, err
	}
	return RunResult{Primary: primary, Candidate: candidate, Skipped: skipped}, nil
}

func buildEstimate(caseID, userID uint, status string, result localization.Result, inputs []localization.Input, skipped []dto.SkippedObservation) (model.LocalizationEstimate, error) {
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
	if skipped == nil {
		skipped = []dto.SkippedObservation{}
	}
	skippedJSON, err := json.Marshal(skipped)
	if err != nil {
		return model.LocalizationEstimate{}, fmt.Errorf("marshal skipped maintenance stations: %w", err)
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
