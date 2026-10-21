package main

import "time"

// A remote outage alone must not produce a restart loop. Recover an unresponsive
// local controller, or stale transports after an observed upstream change.
type mihomoRecovery struct {
	uplink        string
	changed       bool
	localFailures int
	pathFailures  int
	nextAttempt   time.Time
}

func (r *mihomoRecovery) check(now time.Time, uplink string, localOK, tunnelOK bool) string {
	if uplink != "" {
		if r.uplink != "" && uplink != r.uplink {
			r.changed = true
			r.pathFailures = 0
		}
		r.uplink = uplink
	}
	if localOK {
		r.localFailures = 0
	} else {
		r.localFailures++
	}
	if tunnelOK {
		r.pathFailures = 0
		r.changed = false
	}
	if !tunnelOK {
		r.pathFailures++
	}
	if now.Before(r.nextAttempt) {
		return ""
	}
	reason := ""
	if r.localFailures >= 3 {
		reason = "local_controller_unresponsive"
	} else if r.changed && !tunnelOK && r.pathFailures >= 3 {
		reason = "upstream_changed"
	}
	if reason != "" {
		r.nextAttempt = now.Add(time.Minute)
		r.changed = false
		r.localFailures = 0
	}
	return reason
}
