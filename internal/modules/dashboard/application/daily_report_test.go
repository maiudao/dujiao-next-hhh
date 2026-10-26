package application

import (
	"context"
	dashboard "github.com/dujiao-next/internal/modules/dashboard/contract"
	"testing"
	"time"
)

type dailyWindowRepo struct {
	dashboardServiceRepoStub
	start, end time.Time
}

func (s *dailyWindowRepo) BuildDailyExport(_ context.Context, start, end time.Time, _ bool, _ int64) (*dashboard.DailyExport, error) {
	s.start = start
	s.end = end
	return &dashboard.DailyExport{}, nil
}
func TestDailyReportRequiresFullBeijingDay(t *testing.T) {
	repo := &dailyWindowRepo{}
	s := NewService(repo, nil)
	for _, date := range []string{"2026-02-30", "2026-1-2", time.Now().In(time.FixedZone("Asia/Shanghai", 28800)).Format("2006-01-02"), "2999-01-01"} {
		if _, err := s.DailyReport(context.Background(), date); err == nil {
			t.Fatalf("invalid/full-day %s", date)
		}
	}
	report, err := s.DailyReport(context.Background(), "2026-10-25")
	if err != nil {
		t.Fatal(err)
	}
	if repo.start.UTC().Format(time.RFC3339) != "2026-10-24T16:00:00Z" || repo.end.Sub(repo.start) != 24*time.Hour || report.Timezone != "Asia/Shanghai" || report.SchemaVersion != 1 {
		t.Fatal("date/schema contract")
	}
}
