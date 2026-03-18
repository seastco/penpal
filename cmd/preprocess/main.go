// preprocess reads the simplemaps US cities CSV and outputs a precomputed
// city graph with KNN neighbor connections for the routing engine.
//
// Usage:
//
//	go run ./cmd/preprocess -input data/uscities.csv -output data/graph.json
//
// The input CSV should have columns: city, state_id, lat, lng, population
// Download from: https://simplemaps.com/data/us-cities (free basic version)
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stove/penpal/internal/models"
	"github.com/stove/penpal/internal/routing"
)

func main() {
	input := flag.String("input", "data/uscities.csv", "path to simplemaps CSV or cities JSON")
	intlFile := flag.String("international", "", "path to international cities JSON (optional)")
	output := flag.String("output", "data/graph.json", "output path for precomputed graph")
	minPop := flag.Int("min-pop", 0, "minimum population filter (0 = no filter)")
	mergePop := flag.String("merge-pop", "", "path to JSON with population data to merge (by city+state)")
	excludeStates := flag.String("exclude-states", "PR", "comma-separated state codes to exclude")
	flag.Parse()

	excludeSet := make(map[string]bool)
	if *excludeStates != "" {
		for _, s := range strings.Split(*excludeStates, ",") {
			excludeSet[strings.TrimSpace(s)] = true
		}
	}

	log.Printf("reading cities from %s", *input)
	var cities []routing.City
	var err error
	if filepath.Ext(*input) == ".json" {
		cities, err = readJSON(*input)
	} else {
		cities, err = readCSV(*input, *minPop)
	}
	if err != nil {
		log.Fatalf("reading input: %v", err)
	}

	// Filter excluded states
	if len(excludeSet) > 0 {
		var filtered []routing.City
		for _, c := range cities {
			if !excludeSet[c.State] {
				filtered = append(filtered, c)
			}
		}
		log.Printf("excluded %d cities from states %v", len(cities)-len(filtered), *excludeStates)
		cities = filtered
	}

	// Merge population data from another file if provided
	if *mergePop != "" {
		popCities, err := readJSON(*mergePop)
		if err != nil {
			log.Fatalf("reading population file: %v", err)
		}
		merged := mergePopulation(cities, popCities)
		log.Printf("merged population data for %d cities from %s", merged, *mergePop)
	}

	// Merge international cities if provided
	if *intlFile != "" {
		intlCities, err := readJSON(*intlFile)
		if err != nil {
			log.Fatalf("reading international cities: %v", err)
		}
		log.Printf("adding %d international cities", len(intlCities))
		cities = append(cities, intlCities...)
	}

	log.Printf("total: %d cities", len(cities))

	log.Printf("building neighbor graph (K=8)... this may take a minute for 30K cities")
	start := time.Now()
	graph := routing.NewGraph(cities)
	log.Printf("graph built in %v", time.Since(start))

	// Add air bridges to connect disconnected subgraphs (AK, HI, international)
	graph.AddAirBridges(routing.DefaultAirBridges)
	log.Printf("added air bridges")

	// Verify connectivity with a sample route
	if len(cities) > 1 {
		// Try routing between first and last city
		hops, dist, err := graph.Route(0, len(cities)-1, models.TierPriority, time.Now())
		if err != nil {
			log.Printf("warning: sample route failed: %v", err)
		} else {
			log.Printf("sample route: %s -> %s = %.0f mi, %d hops",
				cities[0].FullName(), cities[len(cities)-1].FullName(), dist, len(hops))
		}
	}

	log.Printf("saving precomputed graph to %s", *output)
	if err := graph.SavePrecomputed(*output); err != nil {
		log.Fatalf("saving graph: %v", err)
	}

	// Print file size
	info, _ := os.Stat(*output)
	if info != nil {
		log.Printf("graph file size: %.1f MB", float64(info.Size())/(1024*1024))
	}

	log.Println("done")
}

func readCSV(path string, minPop int) ([]routing.City, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	// Find column indices
	cols := make(map[string]int)
	for i, h := range header {
		cols[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Required columns
	cityCol, ok := cols["city"]
	if !ok {
		// Try alternate names
		for _, name := range []string{"city", "name", "city_ascii"} {
			if idx, found := cols[name]; found {
				cityCol = idx
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("no 'city' column found in CSV (have: %v)", header)
	}

	stateCol := -1
	for _, name := range []string{"state_code", "state_id", "state", "state_name"} {
		if idx, found := cols[name]; found {
			stateCol = idx
			break
		}
	}

	latCol := -1
	for _, name := range []string{"lat", "latitude"} {
		if idx, found := cols[name]; found {
			latCol = idx
			break
		}
	}
	lngCol := -1
	for _, name := range []string{"lng", "longitude", "lon"} {
		if idx, found := cols[name]; found {
			lngCol = idx
			break
		}
	}

	popCol := -1
	for _, name := range []string{"population", "pop"} {
		if idx, found := cols[name]; found {
			popCol = idx
			break
		}
	}

	if latCol == -1 || lngCol == -1 {
		return nil, fmt.Errorf("CSV must have lat/lng columns (have: %v)", header)
	}

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV records: %w", err)
	}

	var cities []routing.City
	seen := make(map[string]bool) // deduplicate by "City, State"

	for _, rec := range records {
		city := strings.TrimSpace(rec[cityCol])
		state := ""
		if stateCol >= 0 && stateCol < len(rec) {
			state = strings.TrimSpace(rec[stateCol])
		}

		lat, err := strconv.ParseFloat(strings.TrimSpace(rec[latCol]), 64)
		if err != nil {
			continue
		}
		lng, err := strconv.ParseFloat(strings.TrimSpace(rec[lngCol]), 64)
		if err != nil {
			continue
		}

		pop := 0
		if popCol >= 0 && popCol < len(rec) {
			p, _ := strconv.ParseFloat(strings.TrimSpace(rec[popCol]), 64)
			pop = int(p)
		}

		if minPop > 0 && pop < minPop {
			continue
		}

		key := city + ", " + state
		if seen[key] {
			continue
		}
		seen[key] = true

		cities = append(cities, routing.City{
			Name:       city,
			State:      state,
			Lat:        lat,
			Lng:        lng,
			Population: pop,
		})
	}

	return cities, nil
}

// normalizeName handles Saint/St. variations for matching.
func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "st.", "saint")
	return name
}

// baseCityName extracts the core city name from Census-style consolidated
// government names like "Nashville-Davidson metropolitan government (balance)"
// or "Indianapolis city (balance)" → "nashville", "indianapolis".
func baseCityName(name string) string {
	name = strings.ToLower(name)
	// Strip parenthetical suffixes: "Milford city (balance)" → "Milford city"
	if idx := strings.Index(name, "("); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	// Strip " city", " town", etc. suffixes added by Census
	for _, suffix := range []string{" city", " town", " village", " borough"} {
		name = strings.TrimSuffix(name, suffix)
	}
	// Take first part before hyphen/slash: "Nashville-Davidson..." → "Nashville"
	if idx := strings.IndexAny(name, "-/"); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	return normalizeName(name)
}

// mergePopulation updates cities in-place with population data from popCities.
// Matches by normalized city name + state code. Also indexes Census consolidated
// government names by their base city name so "Nashville-Davidson..." matches "Nashville".
// Returns number of cities updated.
func mergePopulation(cities []routing.City, popCities []routing.City) int {
	lookup := make(map[string]int) // "name|state" -> population
	for _, c := range popCities {
		if c.Population > 0 {
			state := strings.ToLower(c.State)
			// Index by exact normalized name
			key := normalizeName(c.Name) + "|" + state
			if existing, ok := lookup[key]; !ok || c.Population > existing {
				lookup[key] = c.Population
			}
			// Also index by base city name for Census consolidated names
			base := baseCityName(c.Name)
			if base != normalizeName(c.Name) {
				baseKey := base + "|" + state
				if existing, ok := lookup[baseKey]; !ok || c.Population > existing {
					lookup[baseKey] = c.Population
				}
			}
		}
	}
	merged := 0
	for i := range cities {
		if cities[i].Population > 0 {
			continue
		}
		key := normalizeName(cities[i].Name) + "|" + strings.ToLower(cities[i].State)
		if pop, ok := lookup[key]; ok {
			cities[i].Population = pop
			merged++
		}
	}
	return merged
}

func readJSON(path string) ([]routing.City, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cities []routing.City
	if err := json.Unmarshal(data, &cities); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return cities, nil
}
