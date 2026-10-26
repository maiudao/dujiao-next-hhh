package contract

import "time"

// DailyExport is deliberately incapable of carrying customers, orders,
// fulfillment payloads, payment credentials or administrator sessions.
type DailyExport struct {
	SchemaVersion int            `json:"schema_version"`
	Date          string         `json:"date"`
	Timezone      string         `json:"timezone"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Modules       []DailySection `json:"modules"`
}
type DailySection struct {
	Key   string   `json:"key"`
	State string   `json:"state"`
	Lines []string `json:"lines"`
}
