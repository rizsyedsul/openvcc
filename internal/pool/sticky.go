package pool

import (
	"hash/fnv"
	"net/http"
	"strings"
)

// HashFunc derives an opaque, deterministic-per-client identifier from a
// request. Empty means "no identifier; fall through to the fallback strategy".
type HashFunc func(*http.Request) string

// StickyByCloud pins a client to one cloud once it lands there. The HashFunc
// names the client (cookie value, header value, or remote address); the first
// successful pick for that client is via the fallback strategy, and every
// subsequent pick honours the cloud the client previously landed on.
//
// When the client's preferred cloud is no available healthy backend, the
// strategy falls back so traffic still flows.
type StickyByCloud struct {
	hash       HashFunc
	cookieName string
	fallback   Strategy
}

func NewStickyByCloud(hash HashFunc, cookieName string, fallback Strategy) *StickyByCloud {
	return &StickyByCloud{hash: hash, cookieName: cookieName, fallback: fallback}
}

func (*StickyByCloud) Name() string { return StrategyStickyByCloud }

// CookieName returns the cookie this strategy advertises (for the proxy to
// emit Set-Cookie). Empty when sticky is hash-based (header/remote_addr).
func (s *StickyByCloud) CookieName() string { return s.cookieName }

func (s *StickyByCloud) Pick(req *http.Request, healthy []*Backend) (*Backend, bool) {
	if len(healthy) == 0 {
		return nil, false
	}
	if s.cookieName != "" && req != nil {
		if c, err := req.Cookie(s.cookieName); err == nil && c.Value != "" {
			if b := pickByCloud(c.Value, healthy); b != nil {
				return b, true
			}
		}
	} else if req != nil {
		if id := s.hash(req); id != "" {
			cloud := chooseCloudByHash(id, distinctClouds(healthy))
			if b := pickByCloud(cloud, healthy); b != nil {
				return b, true
			}
		}
	}
	if s.fallback != nil {
		return s.fallback.Pick(req, healthy)
	}
	return healthy[0], true
}

func pickByCloud(cloud string, healthy []*Backend) *Backend {
	cloud = strings.TrimSpace(cloud)
	if cloud == "" {
		return nil
	}
	for _, b := range healthy {
		if b.Cloud == cloud {
			return b
		}
	}
	return nil
}

func distinctClouds(healthy []*Backend) []string {
	seen := make(map[string]bool, len(healthy))
	out := make([]string, 0, len(healthy))
	for _, b := range healthy {
		if !seen[b.Cloud] && b.Cloud != "" {
			seen[b.Cloud] = true
			out = append(out, b.Cloud)
		}
	}
	return out
}

func chooseCloudByHash(id string, clouds []string) string {
	if len(clouds) == 0 {
		return ""
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return clouds[int(h.Sum32())%len(clouds)]
}

// HashFromRequest builds a HashFunc from a config string of the form
// "cookie:NAME", "header:NAME", or "remote_addr". The returned cookieName
// is non-empty only for the cookie: variant.
func HashFromRequest(spec string) (HashFunc, string, error) {
	if spec == "remote_addr" {
		return func(r *http.Request) string {
			if r == nil {
				return ""
			}
			return r.RemoteAddr
		}, "", nil
	}
	if strings.HasPrefix(spec, "header:") {
		name := strings.TrimPrefix(spec, "header:")
		return func(r *http.Request) string {
			if r == nil {
				return ""
			}
			return r.Header.Get(name)
		}, "", nil
	}
	if strings.HasPrefix(spec, "cookie:") {
		name := strings.TrimPrefix(spec, "cookie:")
		return func(r *http.Request) string {
			if r == nil {
				return ""
			}
			c, err := r.Cookie(name)
			if err != nil {
				return ""
			}
			return c.Value
		}, name, nil
	}
	return nil, "", errInvalidStickySpec
}

var errInvalidStickySpec = &stickyErr{"invalid sticky.hash; want remote_addr | header:NAME | cookie:NAME"}

type stickyErr struct{ msg string }

func (e *stickyErr) Error() string { return e.msg }
