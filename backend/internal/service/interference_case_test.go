package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/internal/repository"
)

func TestAuthorizeCaseTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    constants.CaseStatus
		to      constants.CaseStatus
		role    string
		allowed bool
	}{
		{"observer starts collection", constants.CaseDraft, constants.CaseCollecting, constants.RoleObserver, true},
		{"observer cannot analyze", constants.CaseCollecting, constants.CaseAnalyzing, constants.RoleObserver, false},
		{"analyst submits review", constants.CaseAnalyzing, constants.CasePendingReview, constants.RoleAnalyst, true},
		{"analyst cannot confirm", constants.CasePendingReview, constants.CaseConfirmed, constants.RoleAnalyst, false},
		{"reviewer confirms", constants.CasePendingReview, constants.CaseConfirmed, constants.RoleReviewer, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authorizeTransition(tt.from, tt.to, tt.role)
			if tt.allowed && err != nil {
				t.Fatalf("expected allowed, got %v", err)
			}
			if !tt.allowed && err == nil {
				t.Fatal("expected transition to be denied")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 维护窗口校验、重叠拒绝与自动恢复测试
// ---------------------------------------------------------------------------

const maintenanceTestDSN = "file:maintenance-service-test?mode=memory&cache=shared"

func newMaintenanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(maintenanceTestDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ReceiverStation{}, &model.MaintenanceWindow{}, &model.BearingObservation{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func TestValidateWindowRange(t *testing.T) {
	base := time.Now().UTC()
	tests := []struct {
		name    string
		start   time.Time
		end     time.Time
		wantErr bool
	}{
		{"valid range", base, base.Add(2 * time.Hour), false},
		{"end before start", base.Add(time.Hour), base, true},
		{"end equals start", base, base, true},
		{"shorter than one minute", base, base.Add(30 * time.Second), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWindowRange(tt.start, tt.end)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestMaintenanceWindowOverlapRejected(t *testing.T) {
	db := newMaintenanceTestDB(t)
	station := model.ReceiverStation{StationCode: "RX-MAINT", Name: "维护测试站", Latitude: 31.2, Longitude: 121.4, AccuracyDeg: 1, StationStatus: "active"}
	if err := db.Create(&station).Error; err != nil {
		t.Fatalf("create station: %v", err)
	}
	repo := repository.NewMaintenanceRepository(db)
	stationRepo := repository.NewStationRepository(db)
	svc := NewMaintenanceService(repo, stationRepo)
	actor := repository.Actor{UserID: 1, Email: "analyst@spectrum.local", Role: "analyst", RequestID: "req-test"}
	ctx := context.Background()
	now := time.Now().UTC()

	first, err := svc.Register(ctx, dto.CreateMaintenanceWindowRequest{
		StationID: station.ID,
		StartAt:   timePointer(now.Add(time.Hour)),
		EndAt:     timePointer(now.Add(3 * time.Hour)),
		Reason:    "天线检修",
	}, actor)
	if err != nil {
		t.Fatalf("register first window: %v", err)
	}

	if _, err := svc.Register(ctx, dto.CreateMaintenanceWindowRequest{
		StationID: station.ID,
		StartAt:   timePointer(now.Add(2 * time.Hour)),
		EndAt:     timePointer(now.Add(4 * time.Hour)),
		Reason:    "重叠窗口",
	}, actor); err == nil {
		t.Fatal("expected overlapping window to be rejected")
	}

	// 端点相接（半开区间）允许保存。
	if _, err := svc.Register(ctx, dto.CreateMaintenanceWindowRequest{
		StationID: station.ID,
		StartAt:   timePointer(first.EndAt),
		EndAt:     timePointer(first.EndAt.Add(time.Hour)),
		Reason:    "接续窗口",
	}, actor); err != nil {
		t.Fatalf("back-to-back window should be allowed: %v", err)
	}

	// 修改成与第二个窗口（3h..4h）重叠应被拒绝。
	if _, err := svc.Update(ctx, first.ID, dto.UpdateMaintenanceWindowRequest{
		StartAt: timePointer(now.Add(2 * time.Hour)),
		EndAt:   timePointer(now.Add(3*time.Hour + 30*time.Minute)),
		Reason:  "调整到重叠时段",
	}, actor); err == nil {
		t.Fatal("expected overlapping update to be rejected")
	}

	// 在自身原区间内调整不与其他窗口重叠，允许保存。
	if _, err := svc.Update(ctx, first.ID, dto.UpdateMaintenanceWindowRequest{
		StartAt: timePointer(now.Add(time.Hour)),
		EndAt:   timePointer(now.Add(2 * time.Hour)),
		Reason:  "天线检修完成一半",
	}, actor); err != nil {
		t.Fatalf("non-overlapping update should be allowed: %v", err)
	}
}

func TestActiveWindowHalfOpenAndExpiry(t *testing.T) {
	window := model.MaintenanceWindow{StartAt: time.Unix(1000, 0).UTC(), EndAt: time.Unix(2000, 0).UTC()}
	if window.ActiveAt(time.Unix(999, 0).UTC()) {
		t.Error("window must not be active before start")
	}
	if !window.ActiveAt(time.Unix(1000, 0).UTC()) {
		t.Error("window must be active at start instant")
	}
	if !window.ActiveAt(time.Unix(1500, 0).UTC()) {
		t.Error("window must be active while open")
	}
	if window.ActiveAt(time.Unix(2000, 0).UTC()) {
		t.Error("window must auto-resume at end instant")
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
