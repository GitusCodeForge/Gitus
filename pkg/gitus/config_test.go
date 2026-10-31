package gitus

import (
	"testing"
	"time"
)

func announcementConfig(a GitusMaintenanceAnnouncement) *GitusConfig {
	return &GitusConfig{
		GlobalVisibility: GLOBAL_VISIBILITY_PUBLIC,
		MaintenanceAnnouncement: a,
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil { t.Fatal(err) }
	return v
}

func TestEffectiveGlobalVisibility(t *testing.T) {
	start := mustTime(t, "2026-11-02T02:00:00+08:00")
	end := mustTime(t, "2026-11-02T04:00:00+08:00")
	base := func() GitusMaintenanceAnnouncement {
		return GitusMaintenanceAnnouncement{
			Enabled: true,
			Message: "down for maintenance",
			StartsAt: start.Format(time.RFC3339),
			EndsAt: end.Format(time.RFC3339),
		}
	}

	cases := []struct {
		name string
		a    GitusMaintenanceAnnouncement
		at   time.Time
		want string
	}{
		{"disabled", GitusMaintenanceAnnouncement{}, start, GLOBAL_VISIBILITY_PUBLIC},
		{"no start", GitusMaintenanceAnnouncement{Enabled: true, Message: "x"}, start, GLOBAL_VISIBILITY_PUBLIC},
		{"before start", base(), start.Add(-time.Minute), GLOBAL_VISIBILITY_PUBLIC},
		{"at start defaults to shutdown", base(), start, GLOBAL_VISIBILITY_SHUTDOWN},
		{"maintenance mode", func() GitusMaintenanceAnnouncement {
			a := base(); a.Mode = GLOBAL_VISIBILITY_MAINTENANCE; return a
		}(), start, GLOBAL_VISIBILITY_MAINTENANCE},
		{"before end", base(), end.Add(-time.Minute), GLOBAL_VISIBILITY_SHUTDOWN},
		{"no auto restore after end", base(), end.Add(time.Minute), GLOBAL_VISIBILITY_SHUTDOWN},
		{"auto restore after end", func() GitusMaintenanceAnnouncement {
			a := base(); a.AutoRestore = true; return a
		}(), end.Add(time.Minute), GLOBAL_VISIBILITY_PUBLIC},
		{"auto restore before end", func() GitusMaintenanceAnnouncement {
			a := base(); a.AutoRestore = true; return a
		}(), end.Add(-time.Minute), GLOBAL_VISIBILITY_SHUTDOWN},
		{"auto restore without end", func() GitusMaintenanceAnnouncement {
			a := base(); a.AutoRestore = true; a.EndsAt = ""; return a
		}(), end.Add(time.Minute), GLOBAL_VISIBILITY_SHUTDOWN},
		{"manual announcement with mode stays public", func() GitusMaintenanceAnnouncement {
			a := base(); a.StartsAt = ""; a.Mode = GLOBAL_VISIBILITY_MAINTENANCE; return a
		}(), start, GLOBAL_VISIBILITY_PUBLIC},
		{"bad start", func() GitusMaintenanceAnnouncement {
			a := base(); a.StartsAt = "not a time"; return a
		}(), start, GLOBAL_VISIBILITY_PUBLIC},
	}
	for _, c := range cases {
		got := announcementConfig(c.a).EffectiveGlobalVisibilityAt(c.at)
		if got != c.want {
			t.Errorf("%s: EffectiveGlobalVisibilityAt = %q; want %q", c.name, got, c.want)
		}
	}
}

func TestShouldShowMaintenanceAnnouncement(t *testing.T) {
	start := mustTime(t, "2026-11-02T02:00:00+08:00")
	cases := []struct {
		name string
		a    GitusMaintenanceAnnouncement
		at   time.Time
		want bool
	}{
		{"disabled", GitusMaintenanceAnnouncement{}, start, false},
		{"no message", GitusMaintenanceAnnouncement{Enabled: true, StartsAt: start.Format(time.RFC3339)}, start.Add(-time.Hour), false},
		{"manual announcement", GitusMaintenanceAnnouncement{Enabled: true, Message: "x"}, start, true},
		{"before start", GitusMaintenanceAnnouncement{Enabled: true, Message: "x", StartsAt: start.Format(time.RFC3339)}, start.Add(-time.Minute), true},
		{"at start", GitusMaintenanceAnnouncement{Enabled: true, Message: "x", StartsAt: start.Format(time.RFC3339)}, start, false},
		{"after start", GitusMaintenanceAnnouncement{Enabled: true, Message: "x", StartsAt: start.Format(time.RFC3339)}, start.Add(time.Minute), false},
		{"bad start", GitusMaintenanceAnnouncement{Enabled: true, Message: "x", StartsAt: "bad"}, start, false},
	}
	for _, c := range cases {
		got := announcementConfig(c.a).ShouldShowMaintenanceAnnouncementAt(c.at)
		if got != c.want {
			t.Errorf("%s: ShouldShowMaintenanceAnnouncementAt = %v; want %v", c.name, got, c.want)
		}
	}
}
