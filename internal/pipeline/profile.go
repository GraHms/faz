package pipeline

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GraHms/faz/internal/prompts"
)

// Model IDs on the Claude API.
const (
	Opus   = "claude-opus-5-5"
	Sonnet = "claude-sonnet-5-5"
	Haiku  = "claude-haiku-5-5"
)

// StageConfig picks the model and budget for one stage. MaxTokens includes
// thinking, which is on by default for these models.
type StageConfig struct {
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	MaxTokens int    `json:"max_tokens"`
}

// Profile maps every stage to its configuration.
type Profile map[string]StageConfig

// Profiles are the built in model routings.
//
// quality: Opus where judgment decides the outcome (planning, critique,
// visual review); Sonnet where volume of output dominates (copy, HTML,
// repair) and for intake; Haiku only to route chat messages.
//
// Intake runs on Sonnet in both profiles. It is more than extraction: it
// reads the photos, chooses the questions, and writes the summary the owner
// confirms, so its mistakes reach every later stage. Haiku saved little
// there next to the cost of a wrong first page.
//
// economy: Sonnet everywhere except classify. Use it to compare against
// quality on your own briefs before deciding what Opus is worth to you.
var Profiles = map[string]Profile{
	"quality": {
		prompts.Intake:   {Sonnet, "medium", 8_000},
		prompts.Content:  {Sonnet, "medium", 16_000},
		prompts.Plan:     {Opus, "high", 32_000},
		prompts.Critique: {Opus, "high", 32_000},
		prompts.Build:    {Sonnet, "medium", 64_000},
		prompts.Review:   {Opus, "medium", 16_000},
		prompts.Repair:   {Sonnet, "medium", 64_000},
		prompts.Classify: {Haiku, "low", 4_000},
		prompts.Edit:     {Sonnet, "low", 64_000},
	},
	"economy": {
		prompts.Intake:   {Sonnet, "medium", 8_000},
		prompts.Content:  {Sonnet, "medium", 16_000},
		prompts.Plan:     {Sonnet, "high", 32_000},
		prompts.Critique: {Sonnet, "high", 32_000},
		prompts.Build:    {Sonnet, "medium", 64_000},
		prompts.Review:   {Sonnet, "medium", 16_000},
		prompts.Repair:   {Sonnet, "medium", 64_000},
		prompts.Classify: {Haiku, "low", 4_000},
		prompts.Edit:     {Sonnet, "low", 64_000},
	},
	// balanced: Opus only for the plan, at medium effort, because the plan is
	// where the brief's requirements are read and decided; Sonnet for the rest.
	"balanced": {
		prompts.Intake:   {Sonnet, "medium", 8_000},
		prompts.Content:  {Sonnet, "medium", 16_000},
		prompts.Plan:     {Opus, "medium", 32_000},
		prompts.Critique: {Sonnet, "high", 32_000},
		prompts.Build:    {Sonnet, "medium", 64_000},
		prompts.Review:   {Sonnet, "medium", 16_000},
		prompts.Repair:   {Sonnet, "medium", 64_000},
		prompts.Classify: {Haiku, "low", 4_000},
		prompts.Edit:     {Sonnet, "low", 64_000},
	},
}

// ProfileNames lists the built in profiles.
func ProfileNames() string {
	var names []string
	for n := range Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Validate makes sure every stage is configured.
func (p Profile) Validate() error {
	for _, s := range prompts.Stages {
		c, ok := p[s]
		if !ok || c.Model == "" || c.MaxTokens <= 0 {
			return fmt.Errorf("profile has no usable configuration for stage %q", s)
		}
	}
	return nil
}
