// Package check holds every deterministic check in the pipeline: on the
// design plan before anything is built, on the HTML source, and on what a
// real browser measured. Nothing here calls a model.
package check

// Severity levels, most severe first.
const (
	Blocker = "blocker" // the page is broken or unreadable
	Major   = "major"   // off plan, off brief, or clearly generic
	Minor   = "minor"   // polish
	Info    = "info"    // context for the reviewer, not a defect
)

// Finding is one result of a deterministic check.
type Finding struct {
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Where    string `json:"where,omitempty"`
	Message  string `json:"message"`
}

// Count returns how many findings have the given severity.
func Count(fs []Finding, severity string) int {
	n := 0
	for _, f := range fs {
		if f.Severity == severity {
			n++
		}
	}
	return n
}

// Filter keeps findings at or above a severity.
func Filter(fs []Finding, atLeast string) []Finding {
	rank := map[string]int{Blocker: 3, Major: 2, Minor: 1, Info: 0}
	var out []Finding
	for _, f := range fs {
		if rank[f.Severity] >= rank[atLeast] {
			out = append(out, f)
		}
	}
	return out
}
