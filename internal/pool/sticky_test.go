package pool

import (
	"net/http"
	"net/url"
	"testing"
)

func mkSBackend(name, cloud string) *Backend {
	u, _ := url.Parse("http://" + name)
	return NewBackend(name, u, cloud, "r", 1)
}

func TestStickyByCloud_CookiePinsCloud(t *testing.T) {
	a := mkSBackend("a", "aws")
	b := mkSBackend("b", "azure")
	hash, cookie, err := HashFromRequest("cookie:openvcc_cloud")
	if err != nil {
		t.Fatalf("HashFromRequest: %v", err)
	}
	s := NewStickyByCloud(hash, cookie, &RoundRobin{})

	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "openvcc_cloud", Value: "azure"})
	got, ok := s.Pick(req, []*Backend{a, b})
	if !ok || got != b {
		t.Fatalf("expected azure pick when cookie says azure, got %v", got)
	}

	// cookie absent → fall back to round-robin (deterministic with this small set)
	req2, _ := http.NewRequest("GET", "/", nil)
	got, ok = s.Pick(req2, []*Backend{a, b})
	if !ok {
		t.Fatal("fallback should pick something")
	}
	if got.Name == "" {
		t.Fatal("got empty backend")
	}
}

func TestStickyByCloud_FallsBackWhenPinnedCloudMissing(t *testing.T) {
	a := mkSBackend("a", "aws")
	hash, cookie, _ := HashFromRequest("cookie:openvcc_cloud")
	s := NewStickyByCloud(hash, cookie, &RoundRobin{})

	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "openvcc_cloud", Value: "azure"})
	got, ok := s.Pick(req, []*Backend{a})
	if !ok || got != a {
		t.Fatalf("expected aws fallback when azure unavailable, got %v", got)
	}
}

func TestStickyByCloud_HeaderHashDeterministic(t *testing.T) {
	a := mkSBackend("a", "aws")
	b := mkSBackend("b", "azure")
	hash, cookie, _ := HashFromRequest("header:X-Forwarded-For")
	if cookie != "" {
		t.Fatal("header hash should not have cookie name")
	}
	s := NewStickyByCloud(hash, cookie, &RoundRobin{})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	first, _ := s.Pick(req, []*Backend{a, b})
	for i := 0; i < 10; i++ {
		got, _ := s.Pick(req, []*Backend{a, b})
		if got != first {
			t.Errorf("hash should be deterministic; iter %d got %v want %v", i, got, first)
		}
	}
}

func TestStickyByCloud_RemoteAddrHash(t *testing.T) {
	a := mkSBackend("a", "aws")
	b := mkSBackend("b", "azure")
	hash, cookie, _ := HashFromRequest("remote_addr")
	if cookie != "" {
		t.Fatal("remote_addr should not advertise a cookie name")
	}
	s := NewStickyByCloud(hash, cookie, &RoundRobin{})

	req1, _ := http.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "1.1.1.1:9999"
	req2, _ := http.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "1.1.1.1:9999"
	got1, _ := s.Pick(req1, []*Backend{a, b})
	got2, _ := s.Pick(req2, []*Backend{a, b})
	if got1 != got2 {
		t.Errorf("same remote_addr should hit same cloud: %v vs %v", got1, got2)
	}
}

func TestStickyByCloud_EmptyHealthy(t *testing.T) {
	hash, cookie, _ := HashFromRequest("remote_addr")
	s := NewStickyByCloud(hash, cookie, &RoundRobin{})
	if _, ok := s.Pick(nil, nil); ok {
		t.Error("empty pool should return ok=false")
	}
}

func TestHashFromRequest_Errors(t *testing.T) {
	if _, _, err := HashFromRequest("magic"); err == nil {
		t.Error("expected error on unknown spec")
	}
}
