package routing

import "testing"

func BenchmarkHaversine(b *testing.B) {
	for b.Loop() {
		Haversine(42.36, -71.06, 39.74, -104.99) // Boston -> Denver
	}
}

func BenchmarkDijkstra(b *testing.B) {
	g := NewGraph(testCities())
	b.ResetTimer()
	for b.Loop() {
		g.Path(0, len(g.Cities)-1) // first to last city
	}
}

func BenchmarkSearchCities(b *testing.B) {
	g := NewGraph(testCities())
	b.ResetTimer()
	for b.Loop() {
		g.SearchCities("Den", 5)
	}
}

func BenchmarkNearestCity(b *testing.B) {
	g := NewGraph(testCities())
	b.ResetTimer()
	for b.Loop() {
		g.NearestCity(42.36, -71.06) // Boston-ish
	}
}
