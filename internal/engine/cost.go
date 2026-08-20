package engine

import "time"

// ap-northeast-1のオンデマンド時間単価(USD)の概算値。2026-07時点の参考値であり課金額の保証はしない。
// c7a系は2026-08にPricing APIから取得した値。
var hourlyUSD = map[string]float64{
	"c5.large":    0.107,
	"c5.xlarge":   0.214,
	"c5.2xlarge":  0.428,
	"c6i.large":   0.107,
	"c7a.large":   0.1292,
	"c7a.xlarge":  0.2584,
	"c7a.2xlarge": 0.5168,
	"t3.medium":   0.0544,
	"t3.large":    0.1088,
}

func HourlyUSD(instanceType string) (float64, bool) {
	h, ok := hourlyUSD[instanceType]
	return h, ok
}

func Estimate(since, now time.Time, hourly float64, nodes int) float64 {
	return now.Sub(since).Hours() * hourly * float64(nodes)
}

// EstimateNodes はノードごとの単価で概算費用を合算する。
// ベンチノードだけインスタンスタイプが違う構成があるので、全ノード同一単価では計算できない。
// 1つでも単価を知らないタイプが混じっていたら ok=false を返す(半端な合計は誤解を招くため)。
func EstimateNodes(since, now time.Time, nodes []Node) (float64, bool) {
	total := 0.0
	for _, n := range nodes {
		hourly, ok := HourlyUSD(n.InstanceType)
		if !ok {
			return 0, false
		}
		total += Estimate(since, now, hourly, 1)
	}
	return total, true
}
