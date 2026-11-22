package loci

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// Log messages; loci stats reads them back.
const (
	msgBoundary = "boundary" // a branch switch and the rule's verdict
	msgReplaced = "replaced" // a suggestion nobody acted on before the next one
	msgHandoff  = "handoff"  // a handoff injected into a new session
	msgStale    = "stale"    // a suggestion picked up too late and dropped
	msgFailed   = "hook failed"
)

// logPath is where hooks report: never to the agent, whose output they would pollute.
func logPath(gd string) string { return filepath.Join(gd, "loci", "log") }

// openLog returns a JSON-lines logger appending to .git/loci/log, or a no-op
// logger when the file cannot be opened.
func openLog(gd string) (zerolog.Logger, func()) {
	nop := func() {}
	p := logPath(gd)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return zerolog.Nop(), nop
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return zerolog.Nop(), nop
	}
	return zerolog.New(f).With().Timestamp().Logger(), func() { _ = f.Close() }
}
