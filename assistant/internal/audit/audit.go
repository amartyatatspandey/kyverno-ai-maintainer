// Package audit is the append-only, write-ahead event log (AUDIT.md).
// Events are written BEFORE the action they describe executes.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Secret scrubber: belt-and-braces (structural no-secrets is primary, SANDBOX.md).
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`(?i)authorization:\s*\S+\s+\S+`),
	regexp.MustCompile(`sk-[A-Za-z0-9-]{20,}`),
}

func Scrub(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

type Event struct {
	RunID string         `json:"run_id"`
	Seq   int            `json:"seq"`
	TS    time.Time      `json:"ts"`
	Type  string         `json:"type"`
	Data  map[string]any `json:"data,omitempty"`
}

type Log struct {
	mu    sync.Mutex
	f     *os.File
	runID string
	seq   int
	dir   string
}

// Start opens audit/runs/<runID>/events.jsonl and writes run_started.
func Start(baseDir, runID string, startData map[string]any) (*Log, error) {
	dir := filepath.Join(baseDir, "runs", runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	l := &Log{f: f, runID: runID, dir: dir}
	l.Emit("run_started", startData)
	return l, nil
}

// Emit writes one event, scrubbed, fsynced (write-ahead guarantee).
func (l *Log) Emit(typ string, data map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	ev := Event{RunID: l.runID, Seq: l.seq, TS: time.Now().UTC(), Type: typ, Data: data}
	b, err := json.Marshal(ev)
	if err != nil {
		b = []byte(fmt.Sprintf(`{"run_id":%q,"seq":%d,"type":%q,"marshal_error":%q}`, l.runID, l.seq, typ, err))
	}
	line := Scrub(string(b))
	fmt.Fprintln(l.f, line)
	l.f.Sync()
}

func (l *Log) Finish(outcome string, totals map[string]any) {
	data := map[string]any{"outcome": outcome}
	for k, v := range totals {
		data[k] = v
	}
	l.Emit("run_finished", data)
	sum, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(filepath.Join(l.dir, "summary.json"), []byte(Scrub(string(sum))), 0o644)
	l.f.Close()
}

func (l *Log) Dir() string { return l.dir }

// NewRunID: sortable, greppable, embedded in comment markers.
func NewRunID(entity string) string {
	entity = strings.NewReplacer("/", "", "#", "").Replace(entity)
	return fmt.Sprintf("run_%s_%s", time.Now().UTC().Format("20060102_150405"), entity)
}

// ReadEvents loads a run's events for rendering / eval.
func ReadEvents(baseDir, runID string) ([]Event, error) {
	b, err := os.ReadFile(filepath.Join(baseDir, "runs", runID, "events.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err == nil {
			out = append(out, ev)
		}
	}
	return out, nil
}

// ListRuns returns run IDs, newest last (lexicographic == chronological).
func ListRuns(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(baseDir, "runs"))
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}
