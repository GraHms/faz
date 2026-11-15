package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const sampleStream = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-5-5","usage":{"input_tokens":1200,"output_tokens":1,"cache_read_input_tokens":800,"cache_creation_input_tokens":0}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"private"}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"` + "```" + `json\n{\"ok\": "}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"true}\n` + "```" + `"}}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":350}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

func TestParseStreamKeepsOnlyText(t *testing.T) {
	resp, err := parseStream(strings.NewReader(sampleStream))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.Text, "private") {
		t.Fatal("thinking text leaked into output")
	}
	var v struct{ OK bool }
	if err := DecodeJSON(resp.Text, &v); err != nil || !v.OK {
		t.Fatalf("decode %q: %v", resp.Text, err)
	}
	if resp.Usage.InputTokens != 1200 || resp.Usage.OutputTokens != 350 || resp.Usage.CacheReadInputTokens != 800 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("stop reason %q", resp.StopReason)
	}
}

func TestBuildBody(t *testing.T) {
	a := &Anthropic{CacheTTL: "1h"}
	b, err := a.buildBody(Request{Model: "claude-opus-5-5", MaxTokens: 100, System: "sys", Effort: "high",
		Parts: []Part{{Text: "hello"}, {Image: []byte{1, 2, 3}, MediaType: "image/png"}}})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"temperature", "top_p", "top_k", "thinking", "tool_choice"} {
		if _, ok := m[banned]; ok {
			t.Errorf("body must not send %s", banned)
		}
	}
	if m["output_config"].(map[string]any)["effort"] != "high" {
		t.Error("effort not set in output_config")
	}
	sys := m["system"].([]any)[0].(map[string]any)
	if sys["cache_control"].(map[string]any)["ttl"] != "1h" {
		t.Error("system prompt not cached for 1h")
	}
	content := m["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 2 || content[1].(map[string]any)["type"] != "image" {
		t.Errorf("content = %v", content)
	}
}

func TestRetriesOnOverload(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-version") == "" || r.Header.Get("x-api-key") != "k" {
			t.Error("missing headers")
		}
		_, _ = io.ReadAll(r.Body)
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("retry-after", "0")
			w.WriteHeader(529)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`))
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte(sampleStream))
	}))
	defer srv.Close()
	a := &Anthropic{APIKey: "k", BaseURL: srv.URL, MaxRetries: 2}
	resp, err := a.Complete(context.Background(), Request{Model: "m", MaxTokens: 10, Parts: []Part{{Text: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(resp.Text, "true") {
		t.Fatalf("calls %d text %q", calls, resp.Text)
	}
}

func TestExtractHTML(t *testing.T) {
	doc, err := ExtractHTML("Here you go:\n```html\n<!DOCTYPE html><html><body>x</body></html>\n```")
	if err != nil || !strings.HasPrefix(doc, "<!DOCTYPE html>") {
		t.Fatalf("%q %v", doc, err)
	}
	if _, err := ExtractHTML("```html\n<!DOCTYPE html><html><body>cut off"); err == nil {
		t.Fatal("truncated document should fail")
	}
}

func TestCostHaikuLongTier(t *testing.T) {
	short := Cost("claude-haiku-5-5", Usage{InputTokens: 100_000, OutputTokens: 1_000_000})
	long := Cost("claude-haiku-5-5", Usage{InputTokens: 100_001, OutputTokens: 1_000_000})
	if short >= long || long < 2.5 {
		t.Fatalf("short %.3f long %.3f", short, long)
	}
}
