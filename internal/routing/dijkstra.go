package routing

import (
	"container/heap"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/seastco/penpal/internal/models"
)

// dijkstraState holds reusable allocations for Dijkstra's algorithm.
type dijkstraState struct {
	dist []float64
	prev []int
}

var dijkstraPool = sync.Pool{
	New: func() any { return &dijkstraState{} },
}

// TransitDays returns the estimated delivery time in business days for a given distance and tier.
// Used for shipping estimates (pre-send display).
func TransitDays(dist float64, tier models.ShippingTier, international bool) float64 {
	days := float64(EstimateBusinessDays(dist, tier))
	if international {
		days += tier.CustomsDays()
	}
	return days
}

const maxHops = 10 // Cap route display to this many relay hops

// Route computes the shortest path between two cities and returns the route hops
// with scheduled ETAs based on the shipping tier. When international is true,
// customs delays are added to the transit time.
func (g *Graph) Route(fromIdx, toIdx int, tier models.ShippingTier, departureTime time.Time, international ...bool) ([]models.RouteHop, float64, error) {
	if fromIdx < 0 || fromIdx >= len(g.Cities) {
		return nil, 0, fmt.Errorf("invalid from city index: %d", fromIdx)
	}
	if toIdx < 0 || toIdx >= len(g.Cities) {
		return nil, 0, fmt.Errorf("invalid to city index: %d", toIdx)
	}
	if fromIdx == toIdx {
		// Same-city route: apply facility processing + delivery window
		express := tier.IsExpress()
		senderLoc := g.Cities[fromIdx].Timezone()
		departure := NextProcessingStart(departureTime, senderLoc, express)
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		// One facility dwell then delivery
		dwell := SampleDwellHours(tier.DwellMeanHours(), tier.DwellSigma(), rng)
		ready := AddFacilityHours(departure, dwell, senderLoc, express)
		eta := NextDeliverySlot(ready, senderLoc, express, rng)
		hop := g.makeHop(fromIdx, eta)
		return []models.RouteHop{hop}, 0, nil
	}

	path, totalDist, err := g.dijkstra(fromIdx, toIdx)
	if err != nil {
		return nil, 0, err
	}

	// Sample path down to maxHops if too many cities
	path = samplePath(path, maxHops)

	intl := len(international) > 0 && international[0]
	hops := g.scheduleHops(path, tier, departureTime, totalDist, intl)
	return hops, totalDist, nil
}

// Path computes the shortest path between two cities, returning the sampled
// city indices and total distance. Use this when you need the path without
// scheduling hops (e.g. shipping estimates where Dijkstra is tier-independent).
func (g *Graph) Path(fromIdx, toIdx int) ([]int, float64, error) {
	if fromIdx < 0 || fromIdx >= len(g.Cities) {
		return nil, 0, fmt.Errorf("invalid from city index: %d", fromIdx)
	}
	if toIdx < 0 || toIdx >= len(g.Cities) {
		return nil, 0, fmt.Errorf("invalid to city index: %d", toIdx)
	}
	if fromIdx == toIdx {
		return []int{fromIdx}, 0, nil
	}
	path, totalDist, err := g.dijkstra(fromIdx, toIdx)
	if err != nil {
		return nil, 0, err
	}
	path = samplePath(path, maxHops)
	return path, totalDist, nil
}

// samplePath reduces a path to at most maxN waypoints, keeping the first and
// last city. The interior is divided into buckets and one random city is picked
// from each, so every letter takes a slightly different relay route.
func samplePath(path []int, maxN int) []int {
	if len(path) <= maxN {
		return path
	}
	sampled := make([]int, maxN)
	sampled[0] = path[0]
	sampled[maxN-1] = path[len(path)-1]

	buckets := maxN - 2
	bucketSize := float64(len(path)-2) / float64(buckets)
	for i := 1; i <= buckets; i++ {
		lo := 1 + int(float64(i-1)*bucketSize)
		hi := 1 + int(float64(i)*bucketSize) - 1
		if hi < lo {
			hi = lo
		}
		sampled[i] = path[lo+rand.Intn(hi-lo+1)]
	}
	return sampled
}

// dijkstra finds the shortest path between two nodes, returning the path as
// a list of city indices and the total distance.
func (g *Graph) dijkstra(from, to int) ([]int, float64, error) {
	n := len(g.Cities)

	// Reuse allocations from pool to reduce GC pressure under concurrent sends.
	state := dijkstraPool.Get().(*dijkstraState)
	defer dijkstraPool.Put(state)
	if cap(state.dist) < n {
		state.dist = make([]float64, n)
		state.prev = make([]int, n)
	} else {
		state.dist = state.dist[:n]
		state.prev = state.prev[:n]
	}
	for i := range state.dist {
		state.dist[i] = math.MaxFloat64
		state.prev[i] = -1
	}
	state.dist[from] = 0

	pq := &priorityQueue{}
	heap.Init(pq)
	heap.Push(pq, &pqItem{node: from, dist: 0})

	for pq.Len() > 0 {
		item := heap.Pop(pq).(*pqItem)
		u := item.node
		if u == to {
			break
		}
		if item.dist > state.dist[u] {
			continue
		}
		for _, e := range g.Adjacency[u] {
			newDist := state.dist[u] + e.Distance
			if newDist < state.dist[e.To] {
				state.dist[e.To] = newDist
				state.prev[e.To] = u
				heap.Push(pq, &pqItem{node: e.To, dist: newDist})
			}
		}
	}

	if state.dist[to] == math.MaxFloat64 {
		return nil, 0, fmt.Errorf("no route from %s to %s", g.Cities[from].FullName(), g.Cities[to].FullName())
	}

	// Reconstruct path
	var path []int
	for u := to; u != -1; u = state.prev[u] {
		path = append(path, u)
	}
	// Reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path, state.dist[to], nil
}

// selectFacilityHops picks which path indices are sorting facilities (incur dwell time).
// Always includes first and last; interior hops are evenly distributed.
func selectFacilityHops(pathLen int, maxFacilities int) map[int]bool {
	facilities := make(map[int]bool)
	facilities[0] = true
	facilities[pathLen-1] = true

	if pathLen <= 2 || maxFacilities <= 2 {
		return facilities
	}

	interior := maxFacilities - 2
	if interior >= pathLen-2 {
		// All hops are facility hops
		for i := 0; i < pathLen; i++ {
			facilities[i] = true
		}
		return facilities
	}

	// Distribute interior facility hops evenly
	step := float64(pathLen-2) / float64(interior+1)
	for i := 1; i <= interior; i++ {
		idx := int(math.Round(step * float64(i)))
		if idx < 1 {
			idx = 1
		}
		if idx >= pathLen-1 {
			idx = pathLen - 2
		}
		facilities[idx] = true
	}
	return facilities
}

// scheduleHops assigns ETAs to each hop using the facility-centric model.
// Each facility hop incurs dwell time (log-normal, within business hours).
// Transit waypoints only add transit time. Final hop snaps to a delivery window.
func (g *Graph) scheduleHops(path []int, tier models.ShippingTier, departureTime time.Time, totalDist float64, international bool) []models.RouteHop {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	express := tier.IsExpress()
	detour := tier.RoadDetourFactor()
	speed := tier.TransitSpeedMPH()

	// Determine departure time based on post office cutoff
	senderLoc := g.Cities[path[0]].Timezone()
	departure := NextProcessingStart(departureTime, senderLoc, express)

	hops := make([]models.RouteHop, len(path))
	hops[0] = g.makeHop(path[0], departure)

	if len(path) == 1 {
		return hops
	}

	// Determine which hops are sorting facilities
	facilities := selectFacilityHops(len(path), tier.MaxFacilityHops())

	cursor := departure

	for i := 1; i < len(path); i++ {
		// Transit time from previous hop
		segDist := Haversine(
			g.Cities[path[i-1]].Lat, g.Cities[path[i-1]].Lng,
			g.Cities[path[i]].Lat, g.Cities[path[i]].Lng,
		) * detour
		transitHours := segDist / speed
		if transitHours < 0.5 {
			transitHours = 0.5 // minimum 30 min transit
		}
		// Transit happens continuously (trucks/planes run 24/7)
		cursor = cursor.Add(time.Duration(transitHours * float64(time.Hour)))

		if facilities[i] && i < len(path)-1 {
			// Facility hop: snap to business hours + dwell time
			cityLoc := g.Cities[path[i]].Timezone()
			cursor = SnapToFacilityHours(cursor, cityLoc, express)

			dwell := SampleDwellHours(tier.DwellMeanHours(), tier.DwellSigma(), rng)

			// International customs: add extra dwell at first facility
			if international && i == 1 {
				dwell += tier.CustomsDays() * 24
			}

			cursor = AddFacilityHours(cursor, dwell, cityLoc, express)
		}

		hops[i] = g.makeHop(path[i], cursor)
	}

	// Final hop: snap to delivery window
	destLoc := g.Cities[path[len(path)-1]].Timezone()
	hops[len(path)-1].ETA = NextDeliverySlot(cursor, destLoc, express, rng)

	return hops
}

func (g *Graph) makeHop(cityIdx int, eta time.Time) models.RouteHop {
	c := g.Cities[cityIdx]
	return models.RouteHop{
		City:  c.FullName(),
		Code:  c.Code(),
		Relay: fmt.Sprintf("%s-relay-%02d", c.Code(), rand.Intn(100)),
		Lat:   c.Lat,
		Lng:   c.Lng,
		ETA:   eta,
	}
}

// TotalDistance returns the total haversine distance of a route in miles.
func TotalDistance(hops []models.RouteHop) float64 {
	total := 0.0
	for i := 1; i < len(hops); i++ {
		total += Haversine(hops[i-1].Lat, hops[i-1].Lng, hops[i].Lat, hops[i].Lng)
	}
	return total
}

// --- Priority Queue for Dijkstra ---

type pqItem struct {
	node  int
	dist  float64
	index int
}

type priorityQueue []*pqItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].dist < pq[j].dist }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *priorityQueue) Push(x any) {
	item := x.(*pqItem)
	item.index = len(*pq)
	*pq = append(*pq, item)
}
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[:n-1]
	return item
}
