package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Anthropic calls the Claude Messages API over plain HTTP with streaming.
//
// It uses only the standard library. Streaming keeps long generations (a
// full HTML page plus thinking) clear of request timeouts.
type Anthropic struct {
	APIKey     string
	BaseURL    string       // defaults to https://api.anthropic.com
	HTTP       *http.Client // defaults to a client with no overall timeout; use ctx
	MaxRetries int          // retries on overload and rate limits; default 4
	CacheTTL   string       // "" for the default five minutes, or "1h"
}

const apiVersion = "2023-06-01"

// APIError is an error the API returned, either as an HTTP status or as an
// error event inside the stream.
type APIError struct {
	Status     int
	Type       string
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anthropic api: status %d, %s: %s", e.Status, e.Type, e.Message)
}

// Retryable reports whether waiting and trying again can help.
func (e *APIError) Retryable() bool {
	switch e.Status {
	case 408, 409, 429, 500, 502, 503, 504, 529:
		return true
	}
	return e.Type == "overloaded_error" || e.Type == "api_error" || e.Type == "rate_limit_error"
}

type apiCache struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type apiSystemBlock struct {
	Type         string    `json:"type"`
	Text         string    `json:"text"`
	CacheControl *apiCache `json:"cache_control,omitempty"`
}

type apiMessage struct {
	Role    string           `json:"role"`
	Content []map[string]any `json:"content"`
}

type apiOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type apiRequest struct {
	Model        string           `json:"model"`
	MaxTokens    int              `json:"max_tokens"`
	Stream       bool             `json:"stream"`
	System       []apiSystemBlock `json:"system,omitempty"`
	Messages     []apiMessage     `json:"messages"`
	OutputConfig *apiOutputConfig `json:"output_config,omitempty"`
}

// buildBody turns a Request into the JSON the API expects. The system prompt
// is marked for prompt caching because it is identical across runs. No
// sampling parameters are sent: current models reject non default values.
func (a *Anthropic) buildBody(req Request) ([]byte, error) {
	if req.Model == "" {
		return nil, errors.New("request has no model")
	}
	if req.MaxTokens <= 0 {
		return nil, errors.New("request has no max_tokens")
	}
	content := make([]map[string]any, 0, len(req.Parts))
	for _, p := range req.Parts {
		if len(p.Image) > 0 {
			mt := p.MediaType
			if mt == "" {
				mt = "image/jpeg"
			}
			content = append(content, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": mt,
					"data":       base64.StdEncoding.EncodeToString(p.Image),
				},
			})
			continue
		}
		if strings.TrimSpace(p.Text) == "" {
			continue
		}
		content = append(content, map[string]any{"type": "text", "text": p.Text})
	}
	if len(content) == 0 {
		return nil, errors.New("request has an empty user message")
	}
	body := apiRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Stream:    true,
		Messages:  []apiMessage{{Role: "user", Content: content}},
	}
	if req.System != "" {
		body.System = []apiSystemBlock{{
			Type:         "text",
			Text:         req.System,
			CacheControl: &apiCache{Type: "ephemeral", TTL: a.CacheTTL},
		}}
	}
	if req.Effort != "" {
		body.OutputConfig = &apiOutputConfig{Effort: req.Effort}
	}
	return json.Marshal(body)
}

// Complete sends the request and retries on overload, rate limits and
// transient server errors with exponential backoff and jitter.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	body, err := a.buildBody(req)
	if err != nil {
		return Response{}, err
	}
	maxRetries := a.MaxRetries
	if maxRetries == 0 {
		maxRetries = 4
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		start := time.Now()
		resp, err := a.once(ctx, body)
		resp.Duration = time.Since(start)
		if err == nil {
			if resp.StopReason == "max_tokens" {
				return resp, ErrTruncated
			}
			return resp, nil
		}
		lastErr = err
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.Retryable() || attempt == maxRetries {
			return resp, err
		}
		wait := apiErr.RetryAfter
		if wait == 0 {
			wait = time.Duration(1<<attempt) * time.Second
			wait += time.Duration(rand.Int63n(int64(wait / 2)))
		}
		select {
		case <-ctx.Done():
			return resp, ctx.Err()
		case <-time.After(wait):
		}
	}
	return Response{}, lastErr
}

func (a *Anthropic) once(ctx context.Context, body []byte) (Response, error) {
	base := a.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", apiVersion)
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("accept", "text/event-stream")

	client := a.HTTP
	if client == nil {
		client = &http.Client{}
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		// Network failures are worth one more try.
		return Response{}, &APIError{Status: 503, Type: "network_error", Message: err.Error()}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, decodeHTTPError(httpResp)
	}
	return parseStream(httpResp.Body)
}

func decodeHTTPError(r *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	e := &APIError{Status: r.StatusCode, Type: "http_error", Message: strings.TrimSpace(string(raw))}
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Error.Type != "" {
		e.Type, e.Message = parsed.Error.Type, parsed.Error.Message
	}
	if s := r.Header.Get("retry-after"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	return e
}

// parseStream reads server sent events and keeps only text blocks. Thinking
// blocks are skipped: the pipeline never sends them back.
func parseStream(r io.Reader) (Response, error) {
	var (
		out        Response
		text       strings.Builder
		blockTypes = map[int]string{}
		event      string
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if err := handleEvent(event, data, &out, &text, blockTypes); err != nil {
				return out, err
			}
		case line == "":
			event = ""
		}
	}
	if err := sc.Err(); err != nil {
		return out, &APIError{Status: 503, Type: "stream_error", Message: err.Error()}
	}
	out.Text = text.String()
	return out, nil
}

func handleEvent(event, data string, out *Response, text *strings.Builder, blockTypes map[int]string) error {
	var ev struct {
		Type    string `json:"type"`
		Index   int    `json:"index"`
		Message struct {
			Model string `json:"model"`
			Usage Usage  `json:"usage"`
		} `json:"message"`
		ContentBlock struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content_block"`
		Delta struct {
			Type       string `json:"type"`
			Text       string `json:"text"`
			StopReason string `json:"stop_reason"`
		} `json:"delta"`
		Usage *Usage `json:"usage"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil // ignore keep alive noise
	}
	kind := ev.Type
	if kind == "" {
		kind = event
	}
	switch kind {
	case "message_start":
		out.Model = ev.Message.Model
		out.Usage = ev.Message.Usage
	case "content_block_start":
		blockTypes[ev.Index] = ev.ContentBlock.Type
		if ev.ContentBlock.Type == "text" {
			text.WriteString(ev.ContentBlock.Text)
		}
	case "content_block_delta":
		if ev.Delta.Type == "text_delta" && blockTypes[ev.Index] == "text" {
			text.WriteString(ev.Delta.Text)
		}
	case "message_delta":
		if ev.Delta.StopReason != "" {
			out.StopReason = ev.Delta.StopReason
		}
		if ev.Usage != nil {
			// message_delta carries cumulative counts.
			if ev.Usage.OutputTokens > 0 {
				out.Usage.OutputTokens = ev.Usage.OutputTokens
			}
			if ev.Usage.InputTokens > 0 {
				out.Usage.InputTokens = ev.Usage.InputTokens
			}
			if ev.Usage.CacheReadInputTokens > 0 {
				out.Usage.CacheReadInputTokens = ev.Usage.CacheReadInputTokens
			}
			if ev.Usage.CacheCreationInputTokens > 0 {
				out.Usage.CacheCreationInputTokens = ev.Usage.CacheCreationInputTokens
			}
		}
	case "error":
		return &APIError{Status: 529, Type: ev.Error.Type, Message: ev.Error.Message}
	}
	return nil
}
