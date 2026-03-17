package routing

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

const (
	earthRadiusMi = 3958.8 // Earth radius in miles
	neighborsK    = 8      // Each city connects to its K nearest neighbors
)

// City represents a city node in the routing graph.
type City struct {
	Name       string  `json:"name"`
	State      string  `json:"state"`
	Country    string  `json:"country,omitempty"` // ISO 3166-1 alpha-2; empty = "US"
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	Population int     `json:"population,omitempty"`
}

// FullName returns "City, ST" for US cities, "City, CC" for international.
func (c City) FullName() string {
	suffix := c.State
	if c.Country != "" && c.Country != "US" {
		suffix = c.Country
	}
	return fmt.Sprintf("%s, %s", c.Name, suffix)
}

// EffectiveCountry returns the country code, defaulting to "US" when empty.
func (c City) EffectiveCountry() string {
	if c.Country == "" {
		return "US"
	}
	return c.Country
}

// Code returns a 3-letter lowercase code derived from the city name.
func (c City) Code() string {
	name := strings.ToLower(c.Name)
	// Remove spaces and non-alpha
	var clean []byte
	for _, b := range []byte(name) {
		if b >= 'a' && b <= 'z' {
			clean = append(clean, b)
		}
	}
	if len(clean) >= 3 {
		return string(clean[:3])
	}
	for len(clean) < 3 {
		clean = append(clean, 'x')
	}
	return string(clean)
}

// Edge represents a connection between two cities.
type Edge struct {
	To       int     // index into the cities slice
	Distance float64 // distance in miles
}

// Graph holds the city routing graph.
type Graph struct {
	Cities    []City   `json:"cities"`
	Adjacency [][]Edge `json:"-"` // adjacency list, not serialized
	coordIdx  map[[2]float64]int  // exact (lat,lng) → city index for O(1) lookup
}

// NewGraph builds a routing graph from a list of cities, connecting each city
// to its K nearest neighbors.
func NewGraph(cities []City) *Graph {
	g := &Graph{
		Cities:    cities,
		Adjacency: make([][]Edge, len(cities)),
	}
	g.buildNeighborGraph()
	g.buildCoordIndex()
	return g
}

// buildCoordIndex populates the exact-coordinate lookup map for O(1) NearestCity.
func (g *Graph) buildCoordIndex() {
	g.coordIdx = make(map[[2]float64]int, len(g.Cities))
	for i, c := range g.Cities {
		g.coordIdx[[2]float64{c.Lat, c.Lng}] = i
	}
}

// buildNeighborGraph connects each city to its K nearest neighbors.
// Uses brute-force O(n^2) which is fine for ~30K cities (takes a few seconds at startup).
// For faster startup, use LoadPrecomputedGraph instead.
func (g *Graph) buildNeighborGraph() {
	n := len(g.Cities)
	for i := 0; i < n; i++ {
		g.Adjacency[i] = g.nearestNeighbors(i, neighborsK)
	}
	// Make edges bidirectional
	for i := 0; i < n; i++ {
		for _, e := range g.Adjacency[i] {
			if !g.hasEdge(e.To, i) {
				g.Adjacency[e.To] = append(g.Adjacency[e.To], Edge{To: i, Distance: e.Distance})
			}
		}
	}
}

func (g *Graph) hasEdge(from, to int) bool {
	for _, e := range g.Adjacency[from] {
		if e.To == to {
			return true
		}
	}
	return false
}

func (g *Graph) nearestNeighbors(idx, k int) []Edge {
	type distIdx struct {
		dist float64
		idx  int
	}
	n := len(g.Cities)
	dists := make([]distIdx, 0, n-1)
	for j := 0; j < n; j++ {
		if j == idx {
			continue
		}
		d := Haversine(g.Cities[idx].Lat, g.Cities[idx].Lng, g.Cities[j].Lat, g.Cities[j].Lng)
		dists = append(dists, distIdx{d, j})
	}
	sort.Slice(dists, func(a, b int) bool {
		return dists[a].dist < dists[b].dist
	})
	if k > len(dists) {
		k = len(dists)
	}
	edges := make([]Edge, k)
	for i := 0; i < k; i++ {
		edges[i] = Edge{To: dists[i].idx, Distance: dists[i].dist}
	}
	return edges
}

// FindCity returns the index of the best matching city, or -1 if not found.
// Searches by exact "City, State" match first, then by city name prefix.
func (g *Graph) FindCity(query string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	// Exact match on full name
	for i, c := range g.Cities {
		if strings.ToLower(c.FullName()) == query {
			return i
		}
	}
	// Prefix match on city name
	for i, c := range g.Cities {
		if strings.HasPrefix(strings.ToLower(c.Name), query) {
			return i
		}
	}
	return -1
}

// SearchCities returns cities matching a query prefix, sorted by population descending.
func (g *Graph) SearchCities(query string, limit int) []City {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	type match struct {
		city  City
		score int // lower = better: 0 = exact name, 1 = prefix, 2 = contains
	}
	var matches []match
	for _, c := range g.Cities {
		lower := strings.ToLower(c.Name)
		full := strings.ToLower(c.FullName())
		if lower == query || full == query {
			matches = append(matches, match{c, 0})
		} else if strings.HasPrefix(lower, query) {
			matches = append(matches, match{c, 1})
		} else if strings.Contains(full, query) {
			matches = append(matches, match{c, 2})
		}
	}
	sort.Slice(matches, func(a, b int) bool {
		if matches[a].score != matches[b].score {
			return matches[a].score < matches[b].score
		}
		return matches[a].city.Population > matches[b].city.Population
	})
	result := make([]City, 0, limit)
	for i := 0; i < len(matches) && i < limit; i++ {
		result = append(result, matches[i].city)
	}
	return result
}

// NearestCity returns the index of the city closest to the given coordinates.
// Uses O(1) lookup for exact graph coordinates (the common case), falling back
// to a linear scan for arbitrary coordinates.
func (g *Graph) NearestCity(lat, lng float64) int {
	if idx, ok := g.coordIdx[[2]float64{lat, lng}]; ok {
		return idx
	}
	best := -1
	bestDist := math.MaxFloat64
	for i, c := range g.Cities {
		d := Haversine(lat, lng, c.Lat, c.Lng)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// Haversine calculates the distance in miles between two lat/lng coordinates.
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	lat1r := lat1 * math.Pi / 180
	lat2r := lat2 * math.Pi / 180
	dlat := (lat2 - lat1) * math.Pi / 180
	dlng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1r)*math.Cos(lat2r)*math.Sin(dlng/2)*math.Sin(dlng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMi * c
}

// PrecomputedGraph is the serialization format for the precomputed neighbor graph.
type PrecomputedGraph struct {
	Cities    []City          `json:"cities"`
	Neighbors [][]PrecompEdge `json:"neighbors"`
}

// PrecompEdge is a compact edge representation for serialization.
type PrecompEdge struct {
	To       int     `json:"t"`
	Distance float64 `json:"d"`
}

// SavePrecomputed serializes the graph for fast loading.
func (g *Graph) SavePrecomputed(path string) error {
	pg := PrecomputedGraph{
		Cities:    g.Cities,
		Neighbors: make([][]PrecompEdge, len(g.Adjacency)),
	}
	for i, edges := range g.Adjacency {
		pg.Neighbors[i] = make([]PrecompEdge, len(edges))
		for j, e := range edges {
			pg.Neighbors[i][j] = PrecompEdge{To: e.To, Distance: e.Distance}
		}
	}
	data, err := json.Marshal(pg)
	if err != nil {
		return fmt.Errorf("marshaling graph: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// AirBridge defines a synthetic edge between two gateway cities (e.g., across oceans).
type AirBridge struct {
	From string // "City, ST" or "City, CC"
	To   string
}

// DefaultAirBridges connects disconnected subgraphs (AK, HI, international).
var DefaultAirBridges = []AirBridge{
	{"Honolulu, HI", "Los Angeles, CA"},
	{"Anchorage, AK", "Seattle, WA"},
	{"New York, NY", "Madrid, ES"},
}

// AddAirBridges injects synthetic bidirectional edges between gateway city pairs.
// Missing cities are silently skipped, so the same list works with dev and prod graphs.
func (g *Graph) AddAirBridges(bridges []AirBridge) {
	for _, b := range bridges {
		fromIdx := g.FindCity(b.From)
		toIdx := g.FindCity(b.To)
		if fromIdx == -1 || toIdx == -1 {
			continue
		}
		dist := Haversine(g.Cities[fromIdx].Lat, g.Cities[fromIdx].Lng,
			g.Cities[toIdx].Lat, g.Cities[toIdx].Lng)
		if !g.hasEdge(fromIdx, toIdx) {
			g.Adjacency[fromIdx] = append(g.Adjacency[fromIdx], Edge{To: toIdx, Distance: dist})
		}
		if !g.hasEdge(toIdx, fromIdx) {
			g.Adjacency[toIdx] = append(g.Adjacency[toIdx], Edge{To: fromIdx, Distance: dist})
		}
	}
}

// LoadPrecomputedGraph loads a precomputed graph from disk (fast, no neighbor computation).
func LoadPrecomputedGraph(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading precomputed graph: %w", err)
	}
	var pg PrecomputedGraph
	if err := json.Unmarshal(data, &pg); err != nil {
		return nil, fmt.Errorf("parsing precomputed graph: %w", err)
	}
	g := &Graph{
		Cities:    pg.Cities,
		Adjacency: make([][]Edge, len(pg.Neighbors)),
	}
	for i, edges := range pg.Neighbors {
		g.Adjacency[i] = make([]Edge, len(edges))
		for j, e := range edges {
			g.Adjacency[i][j] = Edge{To: e.To, Distance: e.Distance}
		}
	}
	g.buildCoordIndex()
	return g, nil
}
