// Package history remembers what the studio produced recently, so the plan
// stage can avoid repeating itself across clients. A human designer carries
// this memory without thinking about it; a model has to be handed it.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Entry summarizes one finished design plan.
type Entry struct {
	Time        time.Time `json:"time"`
	Subject     string    `json:"subject"`
	PageKind    string    `json:"page_kind"`
	DisplayFont string    `json:"display_font"`
	BodyFont    string    `json:"body_font"`
	Background  string    `json:"background"`
	Accent      string    `json:"accent"`
	BoldMove    string    `json:"bold_move"`
}

// Load returns the most recent n entries, oldest first. A missing file is an
// empty history, not an error.
func Load(path string, n int) ([]Entry, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			all = append(all, e)
		}
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, sc.Err()
}

// Append adds one entry to the end of the file, creating it if needed.
func Append(path string, e Entry) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
