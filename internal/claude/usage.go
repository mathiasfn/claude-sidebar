package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type DailyActivity struct {
	Date          string `json:"date"`
	MessageCount  int    `json:"messageCount"`
	SessionCount  int    `json:"sessionCount"`
	ToolCallCount int    `json:"toolCallCount"`
}

type StatsCache struct {
	Version       int             `json:"version"`
	DailyActivity []DailyActivity `json:"dailyActivity"`
	TotalSessions int             `json:"totalSessions"`
	TotalMessages int             `json:"totalMessages"`
}

type UsageSummary struct {
	TodayMessages  int
	TodayTools     int
	TodaySessions  int
	WeekMessages   int
	WeekTools      int
	WeekSessions   int
	TotalSessions  int
	TotalMessages  int
}

func GetUsage() *UsageSummary {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return &UsageSummary{}
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".claude", "stats-cache.json"))
	if err != nil {
		return &UsageSummary{}
	}

	var cache StatsCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return &UsageSummary{}
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	weekAgo := now.AddDate(0, 0, -7).Format("2006-01-02")

	summary := &UsageSummary{
		TotalSessions: cache.TotalSessions,
		TotalMessages: cache.TotalMessages,
	}

	for _, day := range cache.DailyActivity {
		if day.Date == today {
			summary.TodayMessages = day.MessageCount
			summary.TodayTools = day.ToolCallCount
			summary.TodaySessions = day.SessionCount
		}
		if day.Date >= weekAgo {
			summary.WeekMessages += day.MessageCount
			summary.WeekTools += day.ToolCallCount
			summary.WeekSessions += day.SessionCount
		}
	}

	return summary
}
