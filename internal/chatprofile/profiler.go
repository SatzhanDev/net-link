// Package chatprofile implements the rule-based Chat Activity Profiler.
//
// "AI" here means threshold classification with fixed rules: the message
// rate is compared against fixed boundaries and the top talker is picked by
// count. No machine learning or external services are involved.
//
// Every computation is a small pure function so it can be unit tested on its
// own. Session ties them together but is NOT safe for concurrent use: the
// caller (the chat server) guards it with its own mutex.
package chatprofile

import (
	"fmt"
	"time"
)

// Pace labels.
const (
	PaceQuiet  = "quiet"
	PaceActive = "active"
	PaceLively = "lively"
)

// Pace boundaries in messages per minute:
//
//	rate <  3        -> quiet
//	3 <= rate <= 10  -> active
//	rate > 10        -> lively
//
// The ranges cover every real number, so there is no gap between labels.
const (
	activeFrom = 3.0
	activeTo   = 10.0
)

// Flood detection: more than FloodLimit messages within FloodWindow.
const (
	FloodLimit  = 5
	FloodWindow = 30 * time.Second
)

// FloodWarning is sent privately to a client that is flooding the chat.
const FloodWarning = "You are sending messages too fast. Please slow down."

// MessageRate returns messages per minute. A non-positive duration has no
// meaningful rate, so it returns 0 and lets PaceLabel decide.
func MessageRate(total int, elapsedMinutes float64) float64 {
	if elapsedMinutes <= 0 {
		return 0
	}
	return float64(total) / elapsedMinutes
}

// ClassifyRate maps a rate (messages per minute) to a pace label.
func ClassifyRate(rate float64) string {
	switch {
	case rate < activeFrom:
		return PaceQuiet
	case rate <= activeTo:
		return PaceActive
	default:
		return PaceLively
	}
}

// PaceLabel takes total messages and elapsed minutes and returns the pace.
//
// Assumption: if messages exist but no time has passed (elapsed <= 0), the
// rate tends to infinity, so the chat is lively. With no messages the chat
// is always quiet.
func PaceLabel(total int, elapsedMinutes float64) string {
	if total <= 0 {
		return PaceQuiet
	}
	if elapsedMinutes <= 0 {
		return PaceLively
	}
	return ClassifyRate(MessageRate(total, elapsedMinutes))
}

// TopTalker returns the name with the most messages and its count.
// Ties are broken alphabetically so the result does not depend on the
// random map iteration order. An empty map (or only zero counts) returns
// ("", 0).
func TopTalker(counts map[string]int) (string, int) {
	bestName, bestCount := "", 0
	for name, count := range counts {
		if count > bestCount || (count == bestCount && count > 0 && name < bestName) {
			bestName, bestCount = name, count
		}
	}
	return bestName, bestCount
}

// FormatSummary builds the session profile text (without the [System] prefix):
//
//	Session profile | pace: active | top talker: Yenlik (8 msgs) | total: 15 msgs
//	Session profile | pace: quiet | no messages yet
func FormatSummary(pace, topName string, topCount, total int) string {
	if total <= 0 {
		return "Session profile | pace: " + pace + " | no messages yet"
	}
	return fmt.Sprintf("Session profile | pace: %s | top talker: %s (%d msgs) | total: %d msgs",
		pace, topName, topCount, total)
}

// RecentMessages adds now to the timestamps of a client's recent messages
// and drops the ones that fell out of the window. A timestamp stays in the
// window while now - t < window. The input slice is reused.
func RecentMessages(times []time.Time, now time.Time, window time.Duration) []time.Time {
	kept := times[:0]
	for _, t := range times {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	return append(kept, now)
}

// IsFlooding reports whether count messages inside the window exceed limit.
func IsFlooding(count, limit int) bool {
	return count > limit
}

// Session holds the running counters of one chat session.
type Session struct {
	start  time.Time
	total  int
	counts map[string]int
}

// NewSession starts counting at start.
func NewSession(start time.Time) *Session {
	return &Session{start: start, counts: make(map[string]int)}
}

// Record counts one message from name.
func (s *Session) Record(name string) {
	s.total++
	s.counts[name]++
}

// Summary returns the live session profile at the moment now.
func (s *Session) Summary(now time.Time) string {
	elapsed := now.Sub(s.start).Minutes()
	name, count := TopTalker(s.counts)
	return FormatSummary(PaceLabel(s.total, elapsed), name, count, s.total)
}
