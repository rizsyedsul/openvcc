package pool

import (
	"net/http"
	"net/url"
	"testing"
)

func mkKBackend(name, cloud, kind string) *Backend {
	u, _ := url.Parse("http://" + name)
	return NewBackend(name, u, cloud, "r", 1).WithKind(kind)
}

func newKindAware(fallback Strategy) *KindAware {
	return NewKindAware(
		[]string{"POST", "PUT", "DELETE", "PATCH"},
		[]string{"writeable"},
		nil,
		fallback,
	)
}

func TestKindAware_WriteRoutedToWriteable(t *testing.T) {
	w := mkKBackend("w", "aws", "writeable")
	r := mkKBackend("r", "azure", "read_only")
	k := newKindAware(&RoundRobin{})
	req, _ := http.NewRequest("POST", "/", nil)

	for i := 0; i < 5; i++ {
		got, ok := k.Pick(req, []*Backend{w, r})
		if !ok || got != w {
			t.Fatalf("iter %d: write should go to writeable; got %v ok=%v", i, got, ok)
		}
	}
}

func TestKindAware_ReadAcceptsAnyByDefault(t *testing.T) {
	w := mkKBackend("w", "aws", "writeable")
	r := mkKBackend("r", "azure", "read_only")
	k := newKindAware(&RoundRobin{})
	req, _ := http.NewRequest("GET", "/", nil)

	hits := map[string]int{}
	for i := 0; i < 10; i++ {
		got, _ := k.Pick(req, []*Backend{w, r})
		hits[got.Name]++
	}
	if hits["w"] == 0 || hits["r"] == 0 {
		t.Errorf("read should reach both kinds: %v", hits)
	}
}

func TestKindAware_WriteFailsClosedWhenNoMatch(t *testing.T) {
	r := mkKBackend("r", "azure", "read_only")
	k := newKindAware(&RoundRobin{})
	req, _ := http.NewRequest("POST", "/", nil)

	if _, ok := k.Pick(req, []*Backend{r}); ok {
		t.Fatal("expected fail-closed when no writeable backend is healthy")
	}
}

func TestKindAware_ReadFallsBackWhenNoMatch(t *testing.T) {
	w := mkKBackend("w", "aws", "writeable")
	k := NewKindAware(
		[]string{"POST"}, []string{"writeable"}, []string{"cache"}, // restrictive read kinds
		&RoundRobin{})
	req, _ := http.NewRequest("GET", "/", nil)
	got, ok := k.Pick(req, []*Backend{w})
	if !ok || got != w {
		t.Fatalf("read should fall back to all healthy when read_kinds excludes everything")
	}
}

func TestKindAware_CustomMethodsCaseInsensitive(t *testing.T) {
	w := mkKBackend("w", "aws", "writeable")
	r := mkKBackend("r", "azure", "read_only")
	k := NewKindAware([]string{"post"}, []string{"writeable"}, nil, &RoundRobin{})
	req, _ := http.NewRequest("POST", "/", nil)
	got, ok := k.Pick(req, []*Backend{w, r})
	if !ok || got != w {
		t.Fatalf("method matching should be case-insensitive")
	}
}

func TestKindAware_NilRequestUsesFallback(t *testing.T) {
	w := mkKBackend("w", "aws", "writeable")
	k := newKindAware(&RoundRobin{})
	got, ok := k.Pick(nil, []*Backend{w})
	if !ok || got != w {
		t.Fatalf("nil request should pass through to fallback")
	}
}
