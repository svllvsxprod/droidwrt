package main

import "time"

// Remote packet loss is not proof that the local core needs restarting.
// Preserve the session and let failover choose a verified working reserve.
type awgRecovery struct {
	localFailures, healthySamples int
	nextAttempt                   time.Time
	delay                         time.Duration
}

func (r *awgRecovery) check(now time.Time, local, tunnel bool) string {
	if local {
		r.localFailures = 0
	} else {
		r.localFailures++
	}
	if local && tunnel {
		r.healthySamples++
	} else {
		r.healthySamples = 0
	}
	if r.healthySamples >= 5 {
		r.delay = 0
	}
	if r.localFailures < 3 || now.Before(r.nextAttempt) {
		return ""
	}
	if r.delay == 0 {
		r.delay = time.Minute
	} else {
		r.delay *= 2
	}
	if r.delay > 15*time.Minute {
		r.delay = 15 * time.Minute
	}
	r.nextAttempt = now.Add(r.delay)
	r.localFailures = 0
	return "local_core_unresponsive"
}
