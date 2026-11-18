package prayer

import "time"

// DefaultReminderMinutes is the snooze length when none is saved.
const DefaultReminderMinutes = 10

// Reminder decides when the due reminder shows. A new due prayer shows it
// at once; dismissing snoozes it (snooze on) or hides it until the next
// prayer becomes due (snooze off).
type Reminder struct {
	now                  func() time.Time
	Visible              bool
	Current              Time
	HasCurrent           bool
	dismissedPermanently map[int64]bool
	snoozedUntil         time.Time
}

// NewReminder uses the clock now, or the system clock when nil.
func NewReminder(now func() time.Time) *Reminder {
	if now == nil {
		now = time.Now
	}
	return &Reminder{now: now, dismissedPermanently: map[int64]bool{}}
}

// OnTick takes the latest due prayer (ok=false for none).
func (r *Reminder) OnTick(latestDue Time, ok bool, snoozeEnabled bool) {
	now := r.now()
	if !ok {
		r.Visible = false
		r.HasCurrent = false
		r.Current = Time{}
		r.dismissedPermanently = map[int64]bool{}
		r.snoozedUntil = time.Time{}
		return
	}
	if !r.HasCurrent || !r.Current.Equal(latestDue) {
		r.Current, r.HasCurrent = latestDue, true
		r.Visible = true
		return
	}
	if !r.Visible && snoozeEnabled &&
		!r.dismissedPermanently[latestDue.Instant.Unix()] &&
		!now.Before(r.snoozedUntil) {
		r.Visible = true
	}
}

// Dismiss hides the reminder: for minutes when snoozing is on, otherwise
// until another prayer becomes due.
func (r *Reminder) Dismiss(snoozeEnabled bool, minutes int) {
	r.Visible = false
	if snoozeEnabled {
		r.snoozedUntil = r.now().Add(time.Duration(minutes) * time.Minute)
	} else if r.HasCurrent {
		r.dismissedPermanently[r.Current.Instant.Unix()] = true
	}
}
