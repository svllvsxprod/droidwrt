package main

import (
	"math"
	"time"
)

// Each tunnel/address family learns its own end-to-end request latency. Limits
// bound resource use; they are not an unconditional declaration of tunnel loss.
type probeTiming struct {
	Mean, Variation float64
	Samples         int
	Backoff         float64
}

func (p *probeTiming) budget() time.Duration {
	seconds := 4.0
	if p.Samples > 0 {
		seconds = p.Mean + 4*p.Variation
	}
	seconds = math.Max(seconds, p.Backoff)
	return time.Duration(math.Max(3, math.Min(12, seconds)) * float64(time.Second))
}
func (p *probeTiming) observe(elapsed time.Duration, success, timeout bool) {
	if success {
		sample := elapsed.Seconds()
		if p.Samples == 0 {
			p.Mean, p.Variation = sample, sample/2
		} else {
			p.Variation = .75*p.Variation + .25*math.Abs(p.Mean-sample)
			p.Mean = .875*p.Mean + .125*sample
		}
		p.Samples++
		p.Backoff *= .8
	} else if timeout {
		// Failed/timeout samples must not disappear from the learned allowance.
		p.Backoff = math.Min(12, p.budget().Seconds()*1.5)
	}
}

type pathAssessment struct {
	Failures int
	State    string
}

func (p *pathAssessment) observe(local, v4, v6, retried bool) {
	if !local {
		p.Failures++
		p.State = "unresponsive"
		return
	}
	if v4 && v6 {
		p.Failures = 0
		p.State = "connected"
		if retried {
			p.State = "degraded"
		}
		return
	}
	p.Failures++
	p.State = "degraded"
	if p.Failures >= 3 {
		p.State = "unresponsive"
	}
}
