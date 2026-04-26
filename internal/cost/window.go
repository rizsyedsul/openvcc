package cost

import "time"

// ringWindow keeps egress samples and answers "sum of bytes in the last D".
// Buckets at 1-second resolution; capacity capped at 1h to bound memory.
const (
	bucketRes = time.Second
	maxAge    = time.Hour
)

type sample struct {
	at    time.Time
	bytes int64
}

type ringWindow struct {
	samples []sample
}

func newRingWindow() *ringWindow {
	return &ringWindow{samples: make([]sample, 0, 64)}
}

func (r *ringWindow) add(now time.Time, bytes int64) {
	r.evict(now)
	if n := len(r.samples); n > 0 && now.Sub(r.samples[n-1].at) < bucketRes {
		r.samples[n-1].bytes += bytes
		return
	}
	r.samples = append(r.samples, sample{at: now, bytes: bytes})
}

func (r *ringWindow) sum(now time.Time, window time.Duration) int64 {
	r.evict(now)
	cutoff := now.Add(-window)
	var total int64
	for i := len(r.samples) - 1; i >= 0; i-- {
		if r.samples[i].at.Before(cutoff) {
			break
		}
		total += r.samples[i].bytes
	}
	return total
}

func (r *ringWindow) evict(now time.Time) {
	cutoff := now.Add(-maxAge)
	i := 0
	for i < len(r.samples) && r.samples[i].at.Before(cutoff) {
		i++
	}
	if i > 0 {
		r.samples = append(r.samples[:0], r.samples[i:]...)
	}
}
