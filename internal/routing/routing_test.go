package routing

import (
	"testing"
	"time"

	"github.com/stove/penpal/internal/models"
)

// testCities returns a small set of cities for testing.
func testCities() []City {
	return []City{
		{Name: "Boston", State: "MA", Lat: 42.3601, Lng: -71.0589, Population: 675647},
		{Name: "New York", State: "NY", Lat: 40.7128, Lng: -74.0060, Population: 8336817},
		{Name: "Hartford", State: "CT", Lat: 41.7658, Lng: -72.6734, Population: 121054},
		{Name: "Philadelphia", State: "PA", Lat: 39.9526, Lng: -75.1652, Population: 1603797},
		{Name: "Pittsburgh", State: "PA", Lat: 40.4406, Lng: -79.9959, Population: 302407},
		{Name: "Cleveland", State: "OH", Lat: 41.4993, Lng: -81.6944, Population: 372624},
		{Name: "Chicago", State: "IL", Lat: 41.8781, Lng: -87.6298, Population: 2693976},
		{Name: "Des Moines", State: "IA", Lat: 41.5868, Lng: -93.6250, Population: 214237},
		{Name: "Denver", State: "CO", Lat: 39.7392, Lng: -104.9903, Population: 715522},
		{Name: "Los Angeles", State: "CA", Lat: 34.0522, Lng: -118.2437, Population: 3979576},
		{Name: "Detroit", State: "MI", Lat: 42.3314, Lng: -83.0458, Population: 670031},
		{Name: "Indianapolis", State: "IN", Lat: 39.7684, Lng: -86.1581, Population: 887642},
	}
}

func TestNewGraph(t *testing.T) {
	cities := testCities()
	g := NewGraph(cities)

	if len(g.Cities) != len(cities) {
		t.Fatalf("expected %d cities, got %d", len(cities), len(g.Cities))
	}

	// Every city should have at least 1 neighbor
	for i, adj := range g.Adjacency {
		if len(adj) == 0 {
			t.Fatalf("city %d (%s) has no neighbors", i, cities[i].FullName())
		}
	}
}

func TestHaversine(t *testing.T) {
	// Boston to New York: ~190 miles
	d := Haversine(42.3601, -71.0589, 40.7128, -74.0060)
	if d < 180 || d > 210 {
		t.Fatalf("Boston->NYC distance: got %.0f mi, expected ~190", d)
	}

	// Boston to Denver: ~1770 miles
	d = Haversine(42.3601, -71.0589, 39.7392, -104.9903)
	if d < 1700 || d > 1850 {
		t.Fatalf("Boston->Denver distance: got %.0f mi, expected ~1770", d)
	}

	// Same point: 0
	d = Haversine(42.3601, -71.0589, 42.3601, -71.0589)
	if d > 0.01 {
		t.Fatalf("same point distance: got %.6f, expected ~0", d)
	}
}

func TestRoute_BostonToDenver(t *testing.T) {
	cities := testCities()
	g := NewGraph(cities)

	bostonIdx := 0
	denverIdx := 8

	route, dist, err := g.Route(bostonIdx, denverIdx, models.TierPriority, time.Now())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}

	if dist < 1500 || dist > 2500 {
		t.Fatalf("distance: got %.0f mi, expected 1500-2500", dist)
	}

	if len(route) < 3 {
		t.Fatalf("expected at least 3 hops, got %d", len(route))
	}

	// First hop should be Boston
	if route[0].City != "Boston, MA" {
		t.Fatalf("first hop: got %s, expected Boston, MA", route[0].City)
	}

	// Last hop should be Denver
	if route[len(route)-1].City != "Denver, CO" {
		t.Fatalf("last hop: got %s, expected Denver, CO", route[len(route)-1].City)
	}

	// ETAs should be chronological
	for i := 1; i < len(route); i++ {
		if route[i].ETA.Before(route[i-1].ETA) {
			t.Fatalf("hop %d ETA (%v) before hop %d ETA (%v)", i, route[i].ETA, i-1, route[i-1].ETA)
		}
	}

	t.Logf("Route: %d hops, %.0f mi", len(route), dist)
	for _, hop := range route {
		t.Logf("  %s (%s) — %s", hop.City, hop.Relay, hop.ETA.Format("01/02 15:04"))
	}
}

func TestRoute_BostonToNYC(t *testing.T) {
	cities := testCities()
	g := NewGraph(cities)

	route, dist, err := g.Route(0, 1, models.TierExpress, time.Now())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}

	if dist < 150 || dist > 250 {
		t.Fatalf("distance: got %.0f mi, expected 150-250", dist)
	}

	t.Logf("Route: %d hops, %.0f mi", len(route), dist)
	for _, hop := range route {
		t.Logf("  %s (%s) — %s", hop.City, hop.Relay, hop.ETA.Format("01/02 15:04"))
	}
}

func TestRoute_SameCity(t *testing.T) {
	cities := testCities()
	g := NewGraph(cities)

	route, dist, err := g.Route(0, 0, models.TierPriority, time.Now())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}

	if dist != 0 {
		t.Fatalf("same city distance: got %.0f, expected 0", dist)
	}
	if len(route) != 1 {
		t.Fatalf("same city hops: got %d, expected 1", len(route))
	}
}

func TestRoute_ShippingTierTiming(t *testing.T) {
	cities := testCities()
	g := NewGraph(cities)

	now := time.Now()
	routeExpress, _, _ := g.Route(0, 8, models.TierExpress, now)
	routePriority, _, _ := g.Route(0, 8, models.TierPriority, now)
	routeFirstClass, _, _ := g.Route(0, 8, models.TierFirstClass, now)

	expressTime := routeExpress[len(routeExpress)-1].ETA.Sub(now)
	priorityTime := routePriority[len(routePriority)-1].ETA.Sub(now)
	firstClassTime := routeFirstClass[len(routeFirstClass)-1].ETA.Sub(now)

	// Express should be faster than priority, priority faster than first class
	if expressTime >= priorityTime {
		t.Fatalf("express (%v) not faster than priority (%v)", expressTime, priorityTime)
	}
	if priorityTime >= firstClassTime {
		t.Fatalf("priority (%v) not faster than first class (%v)", priorityTime, firstClassTime)
	}

	t.Logf("Express: %v, Priority: %v, First Class: %v", expressTime, priorityTime, firstClassTime)
}

func TestSearchCities(t *testing.T) {
	cities := testCities()
	g := NewGraph(cities)

	results := g.SearchCities("bos", 5)
	if len(results) == 0 {
		t.Fatal("no results for 'bos'")
	}
	if results[0].Name != "Boston" {
		t.Fatalf("first result: got %s, expected Boston", results[0].Name)
	}

	results = g.SearchCities("den", 5)
	if len(results) == 0 {
		t.Fatal("no results for 'den'")
	}
	if results[0].Name != "Denver" {
		t.Fatalf("first result: got %s, expected Denver", results[0].Name)
	}
}

func TestNearestCity(t *testing.T) {
	cities := testCities()
	g := NewGraph(cities)

	// Near Boston
	idx := g.NearestCity(42.36, -71.06)
	if g.Cities[idx].Name != "Boston" {
		t.Fatalf("nearest to Boston coords: got %s", g.Cities[idx].Name)
	}

	// Near Denver
	idx = g.NearestCity(39.74, -104.99)
	if g.Cities[idx].Name != "Denver" {
		t.Fatalf("nearest to Denver coords: got %s", g.Cities[idx].Name)
	}
}

func TestCityCode(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Boston", "bos"},
		{"New York", "new"},
		{"Los Angeles", "los"},
		{"Des Moines", "des"},
	}
	for _, tt := range tests {
		c := City{Name: tt.name}
		if got := c.Code(); got != tt.want {
			t.Errorf("City{%q}.Code() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
