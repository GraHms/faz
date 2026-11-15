package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Fenced returns the body of the first ```lang block in text. The closing
// fence is the last ``` in the text, so fences quoted inside the body (for
// example in an ASCII wireframe) do not cut it short.
func Fenced(text, lang string) (string, bool) {
	open := "```" + lang
	i := strings.Index(text, open)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(open):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		return "", false
	}
	j := strings.LastIndex(rest, "```")
	if j < 0 {
		return strings.TrimSpace(rest), true
	}
	return strings.TrimSpace(rest[:j]), true
}

// DecodeJSON finds a JSON object in model output and decodes it into v. It
// prefers a ```json block and falls back to the outermost braces.
func DecodeJSON(text string, v any) error {
	body, ok := Fenced(text, "json")
	if !ok {
		start := strings.IndexByte(text, '{')
		end := strings.LastIndexByte(text, '}')
		if start < 0 || end <= start {
			return errors.New("no JSON object found in model output")
		}
		body = text[start : end+1]
	}
	if err := json.Unmarshal([]byte(body), v); err != nil {
		return fmt.Errorf("model output is not valid JSON: %w", err)
	}
	return nil
}

// ExtractHTML returns the full HTML document from model output.
func ExtractHTML(text string) (string, error) {
	doc, ok := Fenced(text, "html")
	if !ok {
		lower := strings.ToLower(text)
		start := strings.Index(lower, "<!doctype html")
		end := strings.LastIndex(lower, "</html>")
		if start < 0 || end < start {
			return "", errors.New("no HTML document found in model output")
		}
		doc = text[start : end+len("</html>")]
	}
	lower := strings.ToLower(doc)
	if !strings.HasPrefix(strings.TrimSpace(lower), "<!doctype html") {
		return "", errors.New("HTML document does not start with <!DOCTYPE html>")
	}
	if !strings.Contains(lower, "</html>") {
		return "", errors.New("HTML document has no closing </html>; output may be truncated")
	}
	return doc + "\n", nil
}
