package llm

// Price is US dollars per million tokens.
type Price struct {
	In, Out, CacheWrite, CacheRead float64
	// Some models bill long prompts at a higher rate. Zero means one rate.
	LongThreshold   int
	LongIn, LongOut float64
}

// Prices are list prices for the Claude API as published in November 2026.
// Standard input and output rates come from the model pages. Cache write is
// 1.25 times input for the five minute cache. Cache read rates for Haiku 5.5
// and Fable 5.2 are assumed at 10 percent of input. Verify against
// https://platform.claude.com/docs/en/about-claude/pricing before relying on
// the totals for billing.
var Prices = map[string]Price{
	"claude-fable-5-2":  {In: 8, Out: 40, CacheWrite: 10, CacheRead: 0.80},
	"claude-opus-5-5":   {In: 4, Out: 20, CacheWrite: 5, CacheRead: 0.20},
	"claude-sonnet-5-5": {In: 2, Out: 10, CacheWrite: 2.5, CacheRead: 0.10},
	"claude-haiku-5-5": {In: 0.10, Out: 0.50, CacheWrite: 0.125, CacheRead: 0.01,
		LongThreshold: 100_000, LongIn: 0.50, LongOut: 2.50},
}

// Cost estimates the dollar cost of one call. Thinking tokens are billed as
// output and are already included in OutputTokens.
func Cost(model string, u Usage) float64 {
	p, ok := Prices[model]
	if !ok {
		return 0
	}
	in, out := p.In, p.Out
	prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	if p.LongThreshold > 0 && prompt > p.LongThreshold {
		in, out = p.LongIn, p.LongOut
	}
	const m = 1_000_000.0
	return float64(u.InputTokens)*in/m +
		float64(u.OutputTokens)*out/m +
		float64(u.CacheCreationInputTokens)*p.CacheWrite/m +
		float64(u.CacheReadInputTokens)*p.CacheRead/m
}
