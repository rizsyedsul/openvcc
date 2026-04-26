package pool

import (
	"net/http"
	"strings"
)

// KindAware filters healthy backends by their Kind label based on the request
// method, then delegates to a fallback strategy. Writes (methods in
// writeMethods) are restricted to backends whose Kind is in writeKinds. Reads
// can go to backends whose Kind is in readKinds; an empty readKinds means
// "any kind is acceptable for reads".
//
// Failure modes:
//   - No backend matches a write request: returns ok=false (fail closed,
//     since shipping a write to a read replica would corrupt state).
//   - No backend matches a read request: falls back to the full healthy set
//     so traffic still flows.
type KindAware struct {
	writeMethods stringSet
	writeKinds   stringSet
	readKinds    stringSet
	fallback     Strategy
}

func NewKindAware(writeMethods, writeKinds, readKinds []string, fallback Strategy) *KindAware {
	return &KindAware{
		writeMethods: setOfUpper(writeMethods),
		writeKinds:   setOf(writeKinds),
		readKinds:    setOf(readKinds),
		fallback:     fallback,
	}
}

func (*KindAware) Name() string { return "kind_aware" }

func (k *KindAware) Pick(req *http.Request, healthy []*Backend) (*Backend, bool) {
	if len(healthy) == 0 {
		return nil, false
	}
	if req == nil {
		return k.fallback.Pick(req, healthy)
	}
	isWrite := k.writeMethods[strings.ToUpper(req.Method)]
	allowed := k.filter(healthy, isWrite)
	if len(allowed) == 0 {
		if isWrite {
			return nil, false
		}
		allowed = healthy
	}
	return k.fallback.Pick(req, allowed)
}

func (k *KindAware) filter(healthy []*Backend, isWrite bool) []*Backend {
	out := make([]*Backend, 0, len(healthy))
	for _, b := range healthy {
		if isWrite {
			if k.writeKinds[b.Kind] {
				out = append(out, b)
			}
		} else {
			if len(k.readKinds) == 0 || k.readKinds[b.Kind] {
				out = append(out, b)
			}
		}
	}
	return out
}

type stringSet map[string]bool

func setOf(in []string) stringSet {
	out := make(stringSet, len(in))
	for _, s := range in {
		out[s] = true
	}
	return out
}

func setOfUpper(in []string) stringSet {
	out := make(stringSet, len(in))
	for _, s := range in {
		out[strings.ToUpper(s)] = true
	}
	return out
}
