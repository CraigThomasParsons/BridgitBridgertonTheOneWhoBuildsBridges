// Package contracts — subscribers.go provides concrete EventHandler
// implementations that react to sync lifecycle events.
//
// Subscribers are attached to the Emitter at startup and receive every
// event emitted during the engine run. Each subscriber handles events
// independently — the report subscriber writes to stdout, the log
// subscriber writes to a persistent file.

package contracts

import (
	"fmt"
	"os"
	"strings"

	"bridgit/internal/report"
)

// NewReportSubscriber creates an EventHandler that writes select events
// into the sync report for stdout rendering.
//
// Only actionable events are surfaced in the report — phase boundaries,
// failures, and projected artifacts. Routine events (package received,
// manifest generated) are too noisy for operator-facing output and are
// left to the log subscriber.
func NewReportSubscriber(syncReport *report.Report) EventHandler {
	return func(event Event) {
		switch event.Type {
		case PackageFailed:
			// Surface failures prominently so operators notice pipeline issues.
			syncReport.Add(fmt.Sprintf("EVENT [%s]: %s", event.Type, event.Message))

		case RegistryLookupFailed:
			// Registry mismatches need operator attention to fix the mapping.
			syncReport.Add(fmt.Sprintf("EVENT [%s]: %s", event.Type, event.Message))
		}

		// All other event types are silently skipped in the report.
		// The log subscriber captures everything for debugging.
	}
}

// NewLogSubscriber creates an EventHandler that appends all events to a
// persistent log file at the given path.
//
// The log file is opened in append mode on the first event and kept open
// for the duration of the run. Each event is written as a single line in
// a human-readable format suitable for grep and tail -f.
//
// If the log file cannot be opened, events are silently dropped rather
// than crashing the engine — logging is best-effort, not critical path.
func NewLogSubscriber(logPath string) EventHandler {
	// logFile is lazily initialized on the first event to avoid creating
	// empty log files when no events are emitted.
	var logFile *os.File

	return func(event Event) {
		// Lazy-open the log file on first write.
		if logFile == nil {
			var openError error
			logFile, openError = os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if openError != nil {
				// Cannot open log file — silently drop events rather than
				// crashing the engine. Logging is non-critical.
				return
			}
		}

		// Format the event as a single grep-friendly log line.
		// Example: [2026-04-03T22:15:00Z] [phase.started] fetch: Phase fetch started
		formattedTimestamp := event.Timestamp.Format("2006-01-02T15:04:05Z07:00")
		logLine := fmt.Sprintf("[%s] [%s] %s: %s",
			formattedTimestamp,
			event.Type,
			event.Phase,
			event.Message,
		)

		// Append metadata as key=value pairs if present.
		if len(event.Metadata) > 0 {
			metadataParts := make([]string, 0, len(event.Metadata))
			for metadataKey, metadataValue := range event.Metadata {
				metadataParts = append(metadataParts, fmt.Sprintf("%s=%s", metadataKey, metadataValue))
			}
			logLine += " {" + strings.Join(metadataParts, ", ") + "}"
		}

		// Write the log line with a trailing newline.
		// Errors are silently dropped — same rationale as the open failure.
		fmt.Fprintln(logFile, logLine)
	}
}
