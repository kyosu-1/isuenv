package engine

import "time"

// ap-northeast-1のオンデマンド時間単価(USD)の概算値。2026-07時点の参考値であり課金額の保証はしない。
var hourlyUSD = map[string]float64{
	"c5.large":   0.107,
	"c5.xlarge":  0.214,
	"c5.2xlarge": 0.428,
	"c6i.large":  0.107,
	"t3.medium":  0.0544,
	"t3.large":   0.1088,
}

func HourlyUSD(instanceType string) (float64, bool) {
	h, ok := hourlyUSD[instanceType]
	return h, ok
}

func Estimate(since, now time.Time, hourly float64, nodes int) float64 {
	return now.Sub(since).Hours() * hourly * float64(nodes)
}
