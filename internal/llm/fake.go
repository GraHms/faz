package llm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Fake answers every request with a recorded response read from Dir, one file
// per stage (for example build.txt). It lets the whole pipeline run, render
// and report without an API key or any spend.
type Fake struct {
	Dir   string
	Calls []Request
}

// Complete returns the fixture for req.Stage with token counts estimated at
// four characters per token, so cost reporting has something to show.
func (f *Fake) Complete(_ context.Context, req Request) (Response, error) {
	f.Calls = append(f.Calls, req)
	b, err := os.ReadFile(filepath.Join(f.Dir, req.Stage+".txt"))
	if err != nil {
		return Response{}, fmt.Errorf("fake client has no fixture for stage %q: %w", req.Stage, err)
	}
	in := len(req.System) / 4
	images := 0
	for _, p := range req.Parts {
		in += len(p.Text) / 4
		if len(p.Image) > 0 {
			images++
		}
	}
	in += images * 1600
	return Response{
		Text:       string(b),
		StopReason: "end_turn",
		Model:      req.Model,
		Usage:      Usage{InputTokens: in, OutputTokens: len(b) / 4},
		Duration:   10 * time.Millisecond,
	}, nil
}
