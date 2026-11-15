// Package llm is the pipeline's only door to a language model.
//
// It defines a small Client interface so the pipeline can run against the
// real Messages API or against recorded fixtures, and it prices every call.
package llm

import (
	"context"
	"errors"
	"time"
)

// Part is one piece of the user message: text, or an image with an optional
// caption placed just before it.
type Part struct {
	Text      string
	Image     []byte
	MediaType string // image/jpeg, image/png, image/webp or image/gif
}

// Request is one single turn call to a model.
type Request struct {
	Stage     string // used for logging and by the fake client
	Model     string
	System    string
	Effort    string // low, medium, high, xhigh, max; empty keeps the model default
	MaxTokens int    // includes thinking tokens
	Parts     []Part
}

// Usage mirrors the token counts the API reports.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// Response is the text the model returned, with its cost drivers.
type Response struct {
	Text       string
	StopReason string
	Model      string
	Usage      Usage
	Duration   time.Duration
}

// Client completes one request.
type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// ErrTruncated means the model hit max_tokens before finishing. Raise the
// stage's MaxTokens rather than trying to stitch partial output together.
var ErrTruncated = errors.New("model output stopped at max_tokens")
