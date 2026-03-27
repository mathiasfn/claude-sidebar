package tokens

import (
	"fmt"

	"github.com/mathias/claude-sidebar/internal/claude"
)

// Pricing per 1M tokens (USD)
type ModelPricing struct {
	InputPer1M      float64
	OutputPer1M     float64
	CacheReadPer1M  float64
	CacheWritePer1M float64
}

var DefaultPricing = map[string]ModelPricing{
	"claude-opus-4-6": {
		InputPer1M:      15.00,
		OutputPer1M:     75.00,
		CacheReadPer1M:  1.50,
		CacheWritePer1M: 18.75,
	},
	"claude-sonnet-4-6": {
		InputPer1M:      3.00,
		OutputPer1M:     15.00,
		CacheReadPer1M:  0.30,
		CacheWritePer1M: 3.75,
	},
	"claude-haiku-4-5": {
		InputPer1M:      0.80,
		OutputPer1M:     4.00,
		CacheReadPer1M:  0.08,
		CacheWritePer1M: 1.00,
	},
}

func EstimateCost(usage claude.Usage, model string) float64 {
	pricing, ok := DefaultPricing[model]
	if !ok {
		pricing = DefaultPricing["claude-opus-4-6"]
	}

	cost := float64(usage.InputTokens) / 1_000_000 * pricing.InputPer1M
	cost += float64(usage.OutputTokens) / 1_000_000 * pricing.OutputPer1M
	cost += float64(usage.CacheReadInputTokens) / 1_000_000 * pricing.CacheReadPer1M
	cost += float64(usage.CacheCreationInputTokens) / 1_000_000 * pricing.CacheWritePer1M
	return cost
}

func FormatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
