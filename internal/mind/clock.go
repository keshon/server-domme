package mind

import (
	"fmt"
	"time"
)

// partOfDay names the part of the day, so she sounds like someone awake at
// this hour rather than a service with no clock.
func partOfDay(t time.Time) string {
	switch h := t.Hour(); {
	case h < 5:
		return "the middle of the night"
	case h < 12:
		return "morning"
	case h < 18:
		return "afternoon"
	case h < 23:
		return "evening"
	default:
		return "late evening"
	}
}

// ago renders how long ago something was, the way a person would say it.
func ago(d time.Duration) string {
	switch {
	case d < 2*time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "an hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%d months ago", int(d.Hours()/24/30))
	}
}

// gap renders a delay in answering. Separate from ago, which deals in days:
// a late answer is late by minutes.
func gap(d time.Duration) string {
	switch m := int(d.Minutes()); {
	case m < 1:
		return "a moment"
	case m == 1:
		return "a minute"
	case m < 60:
		return fmt.Sprintf("%d minutes", m)
	default:
		return "over an hour"
	}
}

// when renders a moment's time relative to now: the clock for today, the
// weekday for this week, the date before that.
func when(t, now time.Time) string {
	t = t.In(now.Location())
	switch days := dayDiff(t, now); {
	case days == 0:
		return "today " + t.Format("15:04")
	case days == 1:
		return "yesterday " + t.Format("15:04")
	case days < 7:
		return t.Format("Monday 15:04")
	default:
		return t.Format("2 Jan 15:04")
	}
}

// dueIn renders when an intention comes due.
func dueIn(due, now time.Time) string {
	if due.IsZero() {
		return "whenever"
	}
	if !due.After(now) {
		return "due now"
	}
	due = due.In(now.Location())
	switch dayDiff(now, due) {
	case 0:
		return "later today, " + partOfDay(due)
	case 1:
		return "tomorrow " + partOfDay(due)
	default:
		return due.Format("Monday") + " " + partOfDay(due)
	}
}

// dayDiff is how many calendar days b is after a.
func dayDiff(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.In(a.Location()).Date()
	da := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	db := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}
