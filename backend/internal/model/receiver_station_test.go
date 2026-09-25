package model

import (
	"testing"
	"time"
)

func TestMaintenanceWindowStatusAt(t *testing.T) {
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	start := base.Add(-time.Hour)
	end := base.Add(time.Hour)
	window := MaintenanceWindow{StartAt: start, EndAt: end}

	tests := []struct {
		name string
		now  time.Time
		want MaintenanceWindowStatus
	}{
		{"before start", start.Add(-time.Minute), MaintenanceUpcoming},
		{"at start is active", start, MaintenanceActive},
		{"during window", base, MaintenanceActive},
		{"just before end", end.Add(-time.Second), MaintenanceActive},
		{"at end is recovered", end, MaintenanceEnded},
		{"after end", end.Add(time.Minute), MaintenanceEnded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := window.StatusAt(tt.now); got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaintenanceWindowsOverlap(t *testing.T) {
	t0 := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	firstStart := t0
	firstEnd := t0.Add(2 * time.Hour)

	tests := []struct {
		name        string
		startB      time.Time
		endB        time.Time
		overlapping bool
	}{
		{"touching at boundary is allowed", firstEnd, firstEnd.Add(time.Hour), false},
		{"fully inside", firstStart.Add(30 * time.Minute), firstStart.Add(90 * time.Minute), true},
		{"starts before and ends inside", firstStart.Add(-time.Hour), firstStart.Add(30 * time.Minute), true},
		{"starts inside and ends after", firstEnd.Add(-30 * time.Minute), firstEnd.Add(time.Hour), true},
		{"fully before", firstStart.Add(-3 * time.Hour), firstStart.Add(-2 * time.Hour), false},
		{"fully after", firstEnd.Add(2 * time.Hour), firstEnd.Add(3 * time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaintenanceWindowsOverlap(firstStart, firstEnd, tt.startB, tt.endB); got != tt.overlapping {
				t.Fatalf("overlap = %v, want %v", got, tt.overlapping)
			}
		})
	}
}
