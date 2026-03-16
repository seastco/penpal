package routing

import (
	"os"
	"testing"
	"time"

	"github.com/stove/penpal/internal/models"
)

func TestPrecomputedGraph(t *testing.T) {
	graphPath := "../../data/graph.json"
	if _, err := os.Stat(graphPath); os.IsNotExist(err) {
		t.Skip("graph.json not found — run `go run ./cmd/preprocess` first")
	}

	g, err := LoadPrecomputedGraph(graphPath)
	if err != nil {
		t.Fatalf("loading graph: %v", err)
	}

	t.Logf("loaded %d cities", len(g.Cities))

	// Test some real routes
	routes := []struct {
		from, to string
		minDist  float64
		maxDist  float64
	}{
		{"Boston, MA", "Denver, CO", 1500, 2000},
		{"New York, NY", "Los Angeles, CA", 2400, 2900},
		{"Chicago, IL", "Houston, TX", 900, 1200},
		{"Seattle, WA", "Miami, FL", 2700, 3500},
	}

	for _, r := range routes {
		fromIdx := g.FindCity(r.from)
		toIdx := g.FindCity(r.to)
		if fromIdx == -1 {
			t.Logf("skipping %s -> %s: origin not in graph", r.from, r.to)
			continue
		}
		if toIdx == -1 {
			t.Logf("skipping %s -> %s: destination not in graph", r.from, r.to)
			continue
		}

		hops, dist, err := g.Route(fromIdx, toIdx, models.TierPriority, time.Now())
		if err != nil {
			t.Errorf("%s -> %s: %v", r.from, r.to, err)
			continue
		}

		if dist < r.minDist || dist > r.maxDist {
			t.Errorf("%s -> %s: distance %.0f mi outside expected range [%.0f, %.0f]",
				r.from, r.to, dist, r.minDist, r.maxDist)
		}

		t.Logf("%s -> %s: %.0f mi, %d hops", r.from, r.to, dist, len(hops))
		for _, h := range hops {
			t.Logf("  %s (%s) — %s", h.City, h.Relay, h.ETA.Format("01/02 15:04"))
		}
	}
}
