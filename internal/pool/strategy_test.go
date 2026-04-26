package pool

import (
	"net/url"
	"testing"
)

func backends(t *testing.T, specs ...struct {
	name, cloud string
	weight      int
}) []*Backend {
	t.Helper()
	out := make([]*Backend, 0, len(specs))
	for _, s := range specs {
		u, _ := url.Parse("http://" + s.name)
		out = append(out, NewBackend(s.name, u, s.cloud, "r", s.weight))
	}
	return out
}

func TestParseStrategy(t *testing.T) {
	cases := map[string]string{
		"":                     StrategyWeightedRoundRobin,
		"weighted_round_robin": StrategyWeightedRoundRobin,
		"round_robin":          StrategyRoundRobin,
		"least_connections":    StrategyLeastConnections,
		"random":               StrategyRandom,
		"  RANDOM  ":           StrategyRandom,
	}
	for in, want := range cases {
		s, err := ParseStrategy(in)
		if err != nil {
			t.Errorf("ParseStrategy(%q) error: %v", in, err)
			continue
		}
		if s.Name() != want {
			t.Errorf("ParseStrategy(%q).Name()=%q want %q", in, s.Name(), want)
		}
	}
	if _, err := ParseStrategy("magic"); err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestRoundRobin_Distributes(t *testing.T) {
	bs := backends(t,
		struct {
			name, cloud string
			weight      int
		}{"a", "aws", 1},
		struct {
			name, cloud string
			weight      int
		}{"b", "azure", 1},
	)
	rr := &RoundRobin{}
	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		got, ok := rr.Pick(nil, bs)
		if !ok {
			t.Fatal("Pick failed")
		}
		counts[got.Name]++
	}
	if counts["a"] != 50 || counts["b"] != 50 {
		t.Errorf("RR distribution: %+v", counts)
	}
}

func TestWeightedRoundRobin_RespectsWeights(t *testing.T) {
	bs := backends(t,
		struct {
			name, cloud string
			weight      int
		}{"heavy", "aws", 3},
		struct {
			name, cloud string
			weight      int
		}{"light", "azure", 1},
	)
	w := &WeightedRoundRobin{}
	counts := map[string]int{}
	for i := 0; i < 400; i++ {
		got, _ := w.Pick(nil, bs)
		counts[got.Name]++
	}
	if counts["heavy"] != 300 || counts["light"] != 100 {
		t.Errorf("WRR distribution mismatch: %+v", counts)
	}
}

func TestWeightedRoundRobin_FallsBackWhenAllZero(t *testing.T) {
	bs := backends(t,
		struct {
			name, cloud string
			weight      int
		}{"a", "aws", 0},
		struct {
			name, cloud string
			weight      int
		}{"b", "azure", 0},
	)
	w := &WeightedRoundRobin{}
	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		got, _ := w.Pick(nil, bs)
		counts[got.Name]++
	}
	if counts["a"] != 50 || counts["b"] != 50 {
		t.Errorf("WRR fallback distribution: %+v", counts)
	}
}

func TestLeastConnections_PicksMinInflight(t *testing.T) {
	bs := backends(t,
		struct {
			name, cloud string
			weight      int
		}{"a", "aws", 1},
		struct {
			name, cloud string
			weight      int
		}{"b", "azure", 1},
	)
	bs[0].IncInflight()
	bs[0].IncInflight()
	got, ok := LeastConnections{}.Pick(nil, bs)
	if !ok || got.Name != "b" {
		t.Fatalf("got=%v ok=%v want b", got, ok)
	}
}

func TestStrategies_EmptyHealthy(t *testing.T) {
	for _, s := range []Strategy{
		&RoundRobin{}, &WeightedRoundRobin{}, LeastConnections{}, Random{},
	} {
		if _, ok := s.Pick(nil, nil); ok {
			t.Errorf("%s.Pick(nil) should fail", s.Name())
		}
	}
}
