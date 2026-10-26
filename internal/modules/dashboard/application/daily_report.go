package application

import (
	"context"
	"errors"
	dashboard "github.com/dujiao-next/internal/modules/dashboard/contract"
	"time"
)

type dailyExportReader interface {
	BuildDailyExport(context.Context, time.Time, time.Time, bool, int64) (*dashboard.DailyExport, error)
}

func (s *Service) DailyReport(ctx context.Context, date string) (*dashboard.DailyExport, error) {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	start, err := time.ParseInLocation("2006-01-02", date, zone)
	if err != nil || start.Format("2006-01-02") != date {
		return nil, errors.New("invalid report date")
	}
	today := time.Now().In(zone)
	midnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, zone)
	if !start.Before(midnight) {
		return nil, errors.New("report requires a complete past day")
	}
	if s == nil || s.repo == nil {
		return nil, errors.New("report source unavailable")
	}
	reader, ok := s.repo.(dailyExportReader)
	if !ok {
		return nil, errors.New("report source unsupported")
	}
	setting := s.loadSetting()
	report, err := reader.BuildDailyExport(ctx, start, start.AddDate(0, 0, 1), setting.Accounting.RefundReversesCost, setting.Alert.LowStockThreshold)
	if err != nil {
		return nil, errors.New("report source unavailable")
	}
	report.SchemaVersion = 1
	report.Date = date
	report.Timezone = "Asia/Shanghai"
	report.GeneratedAt = time.Now()
	return report, nil
}
