package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/internal/repository"
)

var testDBCounter atomic.Uint64

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:mwtest%d?mode=memory&cache=shared", testDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.ReceiverStation{}, &model.MaintenanceWindow{},
		&model.InterferenceCase{}, &model.BearingObservation{},
		&model.LocalizationEstimate{}, &model.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(
			"audit_events", "localization_estimates", "bearing_observations",
			"interference_cases", "maintenance_windows", "receiver_stations", "users",
		)
	})
	return db
}

func seedStationAndUser(t *testing.T, db *gorm.DB) (model.ReceiverStation, model.User) {
	t.Helper()
	calibrated := time.Now().UTC().Add(-time.Hour)
	station := model.ReceiverStation{
		StationCode: "RX-T", Name: "测试站", Latitude: 31.23, Longitude: 121.47,
		AccuracyDeg: 1.5, StationStatus: "active", CalibratedAt: &calibrated,
	}
	if err := db.Create(&station).Error; err != nil {
		t.Fatalf("create station: %v", err)
	}
	user := model.User{Email: "analyst@test.local", DisplayName: "分析员", PasswordHash: "x", Role: constants.RoleAnalyst, Active: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return station, user
}

func testActor(user model.User) repository.Actor {
	return repository.Actor{UserID: user.ID, Email: user.Email, Role: user.Role, RequestID: "test-request"}
}

func TestMaintenanceWindowOverlapRejected(t *testing.T) {
	db := newTestDB(t)
	station, user := seedStationAndUser(t, db)
	svc := NewStationService(repository.NewStationRepository(db))
	ctx := context.Background()
	now := time.Now().UTC()

	first := dto.CreateMaintenanceWindowRequest{
		StartAt: now.Add(time.Hour), EndAt: now.Add(3 * time.Hour), Reason: "天线检修",
	}
	if _, err := svc.CreateMaintenanceWindow(ctx, station.ID, first, testActor(user)); err != nil {
		t.Fatalf("create first window: %v", err)
	}

	overlapping := dto.CreateMaintenanceWindowRequest{
		StartAt: now.Add(2 * time.Hour), EndAt: now.Add(4 * time.Hour), Reason: "再次维护",
	}
	if _, err := svc.CreateMaintenanceWindow(ctx, station.ID, overlapping, testActor(user)); err == nil {
		t.Fatal("expected overlapping window to be rejected")
	}

	adjacent := dto.CreateMaintenanceWindowRequest{
		StartAt: now.Add(3 * time.Hour), EndAt: now.Add(5 * time.Hour), Reason: "相邻维护",
	}
	if _, err := svc.CreateMaintenanceWindow(ctx, station.ID, adjacent, testActor(user)); err != nil {
		t.Fatalf("create adjacent window: %v", err)
	}
}

func TestObservationBlockedDuringMaintenance(t *testing.T) {
	db := newTestDB(t)
	station, user := seedStationAndUser(t, db)
	obsSvc := NewObservationService(repository.NewObservationRepository(db), repository.NewStationRepository(db), repository.NewCaseRepository(db))
	caseSvc := NewCaseService(repository.NewCaseRepository(db))
	ctx := context.Background()

	caseRecord, err := caseSvc.Create(ctx, dto.CreateCaseRequest{CaseCode: "RF-T-1", Title: "测试案例", FrequencyCenterHz: 433920000, Priority: "normal"}, testActor(user))
	if err != nil {
		t.Fatalf("create case: %v", err)
	}

	now := time.Now().UTC()
	window := model.MaintenanceWindow{
		StationID: station.ID, StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		Reason: "计划维护", CreatedBy: user.ID,
	}
	if err := db.Create(&window).Error; err != nil {
		t.Fatalf("create active window: %v", err)
	}

	request := dto.CreateObservationRequest{
		StationID: station.ID, CaseID: caseRecord.ID, BearingDeg: 90,
		SignalDBM: -70, FrequencyHz: 433920000, BandwidthHz: 12500, Quality: "good",
	}
	if _, err := obsSvc.Create(ctx, request, testActor(user)); err == nil {
		t.Fatal("expected observation creation to be blocked during maintenance")
	}
}

func TestLocalizationRunSkipsMaintenanceStation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	analyst := model.User{Email: "analyst@test.local", DisplayName: "分析员", PasswordHash: "x", Role: constants.RoleAnalyst, Active: true}
	if err := db.Create(&analyst).Error; err != nil {
		t.Fatalf("create analyst: %v", err)
	}
	now := time.Now().UTC()
	calibrated := now.Add(-time.Hour)

	makeStation := func(code string, lat, lon float64) model.ReceiverStation {
		station := model.ReceiverStation{
			StationCode: code, Name: code, Latitude: lat, Longitude: lon,
			AccuracyDeg: 1.5, StationStatus: "active", CalibratedAt: &calibrated,
		}
		if err := db.Create(&station).Error; err != nil {
			t.Fatalf("create station %s: %v", code, err)
		}
		return station
	}
	west := makeStation("RX-W", 31.2304, 121.4437)
	south := makeStation("RX-S", 31.2104, 121.4737)
	east := makeStation("RX-E", 31.2304, 121.5037)

	caseRecord := model.InterferenceCase{
		CaseCode: "RF-T-9", Title: "定位跳过案例", FrequencyCenterHz: 433920000,
		CaseStatus: constants.CaseAnalyzing, Priority: "normal", OpenedBy: analyst.ID, Version: 1,
	}
	if err := db.Create(&caseRecord).Error; err != nil {
		t.Fatalf("create case: %v", err)
	}

	observed := now.Add(-10 * time.Minute)
	observations := []model.BearingObservation{
		{StationID: west.ID, CaseID: caseRecord.ID, BearingDeg: 89.6, CorrectedBearingDeg: 89.6, SignalDBM: -67, FrequencyHz: 433920000, BandwidthHz: 12500, ObservedAt: observed, Quality: constants.QualityGood, CreatedBy: analyst.ID},
		{StationID: south.ID, CaseID: caseRecord.ID, BearingDeg: 0.5, CorrectedBearingDeg: 0.5, SignalDBM: -71, FrequencyHz: 433920000, BandwidthHz: 12500, ObservedAt: observed, Quality: constants.QualityGood, CreatedBy: analyst.ID},
		{StationID: east.ID, CaseID: caseRecord.ID, BearingDeg: 269.7, CorrectedBearingDeg: 269.7, SignalDBM: -64, FrequencyHz: 433920000, BandwidthHz: 12500, ObservedAt: observed, Quality: constants.QualityFair, CreatedBy: analyst.ID},
	}
	if err := db.Create(&observations).Error; err != nil {
		t.Fatalf("create observations: %v", err)
	}

	window := model.MaintenanceWindow{
		StationID: east.ID, StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		Reason: "东侧站维护", CreatedBy: analyst.ID,
	}
	if err := db.Create(&window).Error; err != nil {
		t.Fatalf("create window: %v", err)
	}

	// 通过服务在第四个站点登记窗口，验证维护登记审计；该站无观测，不影响本次定位。
	other := makeStation("RX-O", 31.2504, 121.4737)
	maintenanceSvc := NewStationService(repository.NewStationRepository(db))
	if _, err := maintenanceSvc.CreateMaintenanceWindow(ctx, other.ID, dto.CreateMaintenanceWindowRequest{
		StartAt: now.Add(2 * time.Hour), EndAt: now.Add(3 * time.Hour), Reason: "北侧站计划维护",
	}, testActor(analyst)); err != nil {
		t.Fatalf("register maintenance window via service: %v", err)
	}

	estimateSvc := NewEstimateService(
		repository.NewEstimateRepository(db),
		repository.NewObservationRepository(db),
		repository.NewCaseRepository(db),
		repository.NewStationRepository(db),
		1000,
	)
	result, err := estimateSvc.Run(ctx, dto.RunLocalizationRequest{CaseID: caseRecord.ID, AllowOutlier: false}, testActor(analyst))
	if err != nil {
		t.Fatalf("run localization: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped observation, got %d", len(result.Skipped))
	}
	if result.Skipped[0].StationCode != "RX-E" {
		t.Fatalf("expected RX-E to be skipped, got %s", result.Skipped[0].StationCode)
	}
	if result.Skipped[0].Reason == "" || result.Skipped[0].WindowEndsAt == "" {
		t.Fatal("skip reason and window end must be recorded")
	}
	if len(result.Primary.UsedObservationIDsJSON) == 0 {
		t.Fatal("primary estimate must record used observations")
	}

	var auditCount int64
	if err := db.Model(&model.AuditEvent{}).Where("action = ?", "localization_estimate.created").Count(&auditCount).Error; err != nil {
		t.Fatalf("count run audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one run audit event, got %d", auditCount)
	}
	var maintenanceAudit int64
	if err := db.Model(&model.AuditEvent{}).Where("entity_type = ?", "maintenance_window").Count(&maintenanceAudit).Error; err != nil {
		t.Fatalf("count maintenance audit: %v", err)
	}
	if maintenanceAudit != 1 {
		t.Fatalf("expected one maintenance audit event, got %d", maintenanceAudit)
	}
}
