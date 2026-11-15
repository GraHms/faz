// Package edit applies find and replace edits returned by the repair stage.
//
// Edits are cheaper than regenerating the page: a few hundred output tokens
// instead of tens of thousands, and untouched code stays byte for byte.
package edit

import (
	"fmt"
	"strings"
)

// Edit replaces one exact substring.
type Edit struct {
	Find    string `json:"find"`
	Replace string `json:"replace"`
}

// Failure records an edit that could not be applied.
type Failure struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
	Find   string `json:"find"`
}

// Result summarizes an Apply call.
type Result struct {
	Applied int       `json:"applied"`
	Failed  []Failure `json:"failed"`
}

// Apply runs edits in order. An edit applies only when its find text occurs
// exactly once; if the exact text is missing, a match that differs only in
// leading and trailing whitespace on each line is accepted.
func Apply(doc string, edits []Edit) (string, Result) {
	var res Result
	for i, e := range edits {
		n := strings.Count(doc, e.Find)
		switch {
		case n == 1:
			doc = strings.Replace(doc, e.Find, e.Replace, 1)
			res.Applied++
		case n > 1:
			res.Failed = append(res.Failed, Failure{i, fmt.Sprintf("find text occurs %d times", n), short(e.Find)})
		default:
			if start, end, ok := looseFind(doc, e.Find); ok {
				doc = doc[:start] + e.Replace + doc[end:]
				res.Applied++
			} else {
				res.Failed = append(res.Failed, Failure{i, "find text not found", short(e.Find)})
			}
		}
	}
	return doc, res
}

// looseFind matches find line by line, ignoring indentation and trailing
// spaces. It returns byte offsets into doc when there is exactly one match.
func looseFind(doc, find string) (int, int, bool) {
	want := trimLines(find)
	if len(want) == 0 {
		return 0, 0, false
	}
	lines := strings.SplitAfter(doc, "\n")
	offsets := make([]int, len(lines)+1)
	for i, l := range lines {
		offsets[i+1] = offsets[i] + len(l)
	}
	matches := 0
	var start, end int
	for i := 0; i+len(want) <= len(lines); i++ {
		ok := true
		for j, w := range want {
			if strings.TrimSpace(lines[i+j]) != w {
				ok = false
				break
			}
		}
		if ok {
			matches++
			start = offsets[i]
			end = offsets[i+len(want)]
			// Keep the final newline outside the replaced span.
			if strings.HasSuffix(doc[start:end], "\n") && !strings.HasSuffix(find, "\n") {
				end--
			}
		}
	}
	return start, end, matches == 1
}

func trimLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.Trim(s, "\n"), "\n") {
		out = append(out, strings.TrimSpace(l))
	}
	return out
}

func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
