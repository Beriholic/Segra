package download

import "time"

// PruneHistory drops the finished records the history keeps no longer: those
// that ended more than keep ago, and all but the keepAtMost newest of them. A
// keep or keepAtMost of zero or less is no limit. The downloads still on their
// way, paused ones included, are not history and stay whatever the settings
// say, and a record that does not say when it ended is only bound by the
// count: a missing time is not old age.
//
// Only the records go. The files the downloads wrote are left where they are:
// a history that tidies itself must never take a download with it, and a
// record the user can no longer see is still a file they can open in the
// folder. It returns how many records went.
func (m *Manager) PruneHistory(keep time.Duration, keepAtMost int) int {
	m.mu.Lock()
	now := time.Now()
	kept := 0
	var gone []string
	// Newest first, the way List shows them, so that a limit keeps the
	// newest records rather than whichever the map happened to offer.
	for i := len(m.order) - 1; i >= 0; i-- {
		t := m.tasks[m.order[i]]
		if t == nil || !t.Status.Finished() {
			continue
		}
		if ended := t.FinishedAt(); keep > 0 && !ended.IsZero() && now.Sub(ended) > keep {
			gone = append(gone, t.ID)
			continue
		}
		// Only what age leaves alone counts against the limit: a record
		// already gone by age should not take a place from a recent one.
		kept++
		if keepAtMost > 0 && kept > keepAtMost {
			gone = append(gone, t.ID)
		}
	}
	m.mu.Unlock()

	for _, id := range gone {
		m.Remove(id, false)
	}
	return len(gone)
}
