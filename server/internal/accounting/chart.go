package accounting

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
)

//go:embed chart.json
var chartData []byte

type ChartAccount struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	ParentCode      string `json:"parentCode"`
	Level           int    `json:"level"`
	Group           string `json:"group"`
	RequiresPartner bool   `json:"requiresPartner"`
	SourceRow       int    `json:"sourceRow"`
}
type ChartTemplate struct {
	Version       string         `json:"version"`
	EffectiveFrom string         `json:"effectiveFrom"`
	Source        string         `json:"source"`
	SHA256        string         `json:"sha256"`
	Accounts      []ChartAccount `json:"accounts"`
}

func ReferenceChart() ChartTemplate {
	var chart ChartTemplate
	if err := json.Unmarshal(chartData, &chart); err != nil {
		panic("invalid embedded account reference")
	}
	return chart
}

func ReferenceChartHash() string { sum := sha256.Sum256(chartData); return hex.EncodeToString(sum[:]) }
