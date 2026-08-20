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

// ベンチノードだけタイプが違う構成では、全ノード同一単価の計算では概算がずれる。
func TestEstimateNodes_SumsPerNodePrices(t *testing.T) {
	since := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	now := since.Add(2 * time.Hour)
	nodes := []Node{
		{Index: 1, InstanceType: "c7a.large", Role: RoleApp},
		{Index: 2, InstanceType: "c7a.xlarge", Role: RoleBench},
	}
	got, ok := EstimateNodes(since, now, nodes)
	if !ok {
		t.Fatal("all types have a known price, want ok=true")
	}
	want := 2*0.1292 + 2*0.2584
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected %f, got %f", want, got)
	}
	if uniform := Estimate(since, now, 0.1292, len(nodes)); math.Abs(got-uniform) < 1e-9 {
		t.Errorf("mixed types must not be estimated at a single price: %f", uniform)
	}
}

func TestEstimateNodes_UnknownTypeIsNotEstimated(t *testing.T) {
	since := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	nodes := []Node{{InstanceType: "c7a.large"}, {InstanceType: "x1e.32xlarge"}}
	if _, ok := EstimateNodes(since, since.Add(time.Hour), nodes); ok {
		t.Error("an unknown instance type must make the whole estimate unavailable")
	}
}

func TestHourlyUSD_BenchInstanceTypes(t *testing.T) {
	// private-isu の推奨ベンチタイプと、--bench-instance-type で指定しがちな1段上のタイプ。
	for _, typ := range []string{"c7a.xlarge", "c7a.2xlarge"} {
		if h, ok := HourlyUSD(typ); !ok || h <= 0 {
			t.Errorf("%s must have a price: %v %v", typ, h, ok)
		}
	}
}
