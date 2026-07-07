package engine

import (
	"math"
	"testing"
	"time"
)

func TestHourlyUSD(t *testing.T) {
	h, ok := HourlyUSD("c5.large")
	if !ok || h <= 0 {
		t.Fatalf("c5.large must have a price: %v %v", h, ok)
	}
	if _, ok := HourlyUSD("x1e.32xlarge"); ok {
		t.Error("unknown type should return ok=false")
	}
}

func TestEstimate(t *testing.T) {
	since := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	now := since.Add(2 * time.Hour)
	got := Estimate(since, now, 0.107, 3)
	want := 2 * 0.107 * 3
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected %f, got %f", want, got)
	}
}
