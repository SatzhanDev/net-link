package chatprofile

import (
	"testing"
	"time"
)

func TestMessageRate(t *testing.T) {
	tests := []struct {
		total   int
		elapsed float64
		want    float64
	}{
		{0, 1, 0},
		{12, 5, 2.4},
		{30, 3, 10},
		{5, 0, 0},
		{5, -1, 0},
	}
	for _, tt := range tests {
		if got := MessageRate(tt.total, tt.elapsed); got != tt.want {
			t.Errorf("MessageRate(%d, %v) = %v, want %v", tt.total, tt.elapsed, got, tt.want)
		}
	}
}

func TestClassifyRateBoundaries(t *testing.T) {
	tests := []struct {
		rate float64
		want string
	}{
		{0, PaceQuiet},
		{2.4, PaceQuiet}, // example from the subject
		{2.999, PaceQuiet},
		{3, PaceActive},
		{7.5, PaceActive},
		{10, PaceActive},
		{10.001, PaceLively},
		{100, PaceLively},
	}
	for _, tt := range tests {
		if got := ClassifyRate(tt.rate); got != tt.want {
			t.Errorf("ClassifyRate(%v) = %q, want %q", tt.rate, got, tt.want)
		}
	}
}

func TestPaceLabel(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		elapsed float64
		want    string
	}{
		{"no messages", 0, 10, PaceQuiet},
		{"no messages, no time", 0, 0, PaceQuiet},
		{"2.4 per minute", 12, 5, PaceQuiet},
		{"exactly 3 per minute", 3, 1, PaceActive},
		{"exactly 10 per minute", 20, 2, PaceActive},
		{"11 per minute", 11, 1, PaceLively},
		{"messages but no elapsed time", 1, 0, PaceLively},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PaceLabel(tt.total, tt.elapsed); got != tt.want {
				t.Errorf("PaceLabel(%d, %v) = %q, want %q", tt.total, tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestTopTalker(t *testing.T) {
	tests := []struct {
		name      string
		counts    map[string]int
		wantName  string
		wantCount int
	}{
		{"nil map", nil, "", 0},
		{"empty map", map[string]int{}, "", 0},
		{"single", map[string]int{"Lee": 2}, "Lee", 2},
		{"clear winner", map[string]int{"Lee": 3, "Yenlik": 8, "Bob": 4}, "Yenlik", 8},
		{"tie is alphabetical", map[string]int{"Lee": 5, "Bob": 5, "Yenlik": 5}, "Bob", 5},
		{"zero counts only", map[string]int{"Lee": 0}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Repeat to make sure the random map order never matters.
			for i := 0; i < 20; i++ {
				name, count := TopTalker(tt.counts)
				if name != tt.wantName || count != tt.wantCount {
					t.Fatalf("TopTalker() = (%q, %d), want (%q, %d)", name, count, tt.wantName, tt.wantCount)
				}
			}
		})
	}
}

func TestFormatSummary(t *testing.T) {
	got := FormatSummary(PaceActive, "Yenlik", 8, 15)
	want := "Session profile | pace: active | top talker: Yenlik (8 msgs) | total: 15 msgs"
	if got != want {
		t.Errorf("FormatSummary() = %q, want %q", got, want)
	}

	got = FormatSummary(PaceQuiet, "", 0, 0)
	want = "Session profile | pace: quiet | no messages yet"
	if got != want {
		t.Errorf("FormatSummary(empty) = %q, want %q", got, want)
	}
}

func TestRecentMessages(t *testing.T) {
	base := time.Date(2020, 1, 20, 15, 0, 0, 0, time.UTC)
	var times []time.Time

	times = RecentMessages(times, base, FloodWindow)
	times = RecentMessages(times, base.Add(10*time.Second), FloodWindow)
	times = RecentMessages(times, base.Add(29*time.Second), FloodWindow)
	if len(times) != 3 {
		t.Fatalf("expected 3 messages in the window, got %d", len(times))
	}

	// At +30s the first message (age exactly 30s) leaves the window.
	times = RecentMessages(times, base.Add(30*time.Second), FloodWindow)
	if len(times) != 3 {
		t.Fatalf("expected 3 messages after sliding, got %d", len(times))
	}
	if !times[0].Equal(base.Add(10 * time.Second)) {
		t.Errorf("oldest kept message = %v, want +10s", times[0])
	}

	// A long pause empties the window.
	times = RecentMessages(times, base.Add(5*time.Minute), FloodWindow)
	if len(times) != 1 {
		t.Errorf("expected only the new message after a pause, got %d", len(times))
	}
}

func TestIsFlooding(t *testing.T) {
	if IsFlooding(5, FloodLimit) {
		t.Error("5 messages must not be flooding")
	}
	if !IsFlooding(6, FloodLimit) {
		t.Error("6 messages must be flooding")
	}
}

func TestFloodScenario(t *testing.T) {
	base := time.Date(2020, 1, 20, 15, 0, 0, 0, time.UTC)
	var times []time.Time
	for i := 1; i <= 6; i++ {
		times = RecentMessages(times, base.Add(time.Duration(i)*time.Second), FloodWindow)
		flooding := IsFlooding(len(times), FloodLimit)
		if want := i > FloodLimit; flooding != want {
			t.Errorf("message %d: flooding = %v, want %v", i, flooding, want)
		}
	}
}

func TestSessionSummary(t *testing.T) {
	start := time.Date(2020, 1, 20, 15, 0, 0, 0, time.UTC)
	s := NewSession(start)

	if got, want := s.Summary(start.Add(time.Minute)), "Session profile | pace: quiet | no messages yet"; got != want {
		t.Errorf("empty Summary() = %q, want %q", got, want)
	}

	for i := 0; i < 8; i++ {
		s.Record("Yenlik")
	}
	for i := 0; i < 7; i++ {
		s.Record("Lee")
	}
	// 15 messages in 3 minutes = 5 per minute -> active.
	got := s.Summary(start.Add(3 * time.Minute))
	want := "Session profile | pace: active | top talker: Yenlik (8 msgs) | total: 15 msgs"
	if got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}
