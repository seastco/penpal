#!/usr/bin/env python3
"""
Merge US Census Bureau population data into cities JSON.

Downloads the Census Bureau's "Annual Estimates of the Resident Population for
Incorporated Places" and merges population counts into the project's city data
by matching on city name + state code.

Usage:
    # Download Census data first:
    # https://www.census.gov/data/tables/time-series/demo/popest/2020s-total-cities-and-towns.html
    # Save as data/census_population.csv (or use --census flag)

    python3 scripts/merge_population.py \
        --cities data/us_cities_continental.json \
        --census data/census_population.csv \
        --output data/us_cities_continental.json

The Census CSV is expected to have columns like:
    NAME (e.g. "Boston city, Massachusetts")
    POPESTIMATE2023 (or whichever year column you want)

The script normalizes city names (strips " city", " town", " village" suffixes,
title-cases, etc.) and matches against the cities JSON by (name, state).

Cities without a Census match keep population=0.
"""

import argparse
import csv
import json
import re
import sys
from pathlib import Path

# Maps full state names to two-letter codes
STATE_CODES = {
    "Alabama": "AL", "Alaska": "AK", "Arizona": "AZ", "Arkansas": "AR",
    "California": "CA", "Colorado": "CO", "Connecticut": "CT", "Delaware": "DE",
    "Florida": "FL", "Georgia": "GA", "Hawaii": "HI", "Idaho": "ID",
    "Illinois": "IL", "Indiana": "IN", "Iowa": "IA", "Kansas": "KS",
    "Kentucky": "KY", "Louisiana": "LA", "Maine": "ME", "Maryland": "MD",
    "Massachusetts": "MA", "Michigan": "MI", "Minnesota": "MN",
    "Mississippi": "MS", "Missouri": "MO", "Montana": "MT", "Nebraska": "NE",
    "Nevada": "NV", "New Hampshire": "NH", "New Jersey": "NJ",
    "New Mexico": "NM", "New York": "NY", "North Carolina": "NC",
    "North Dakota": "ND", "Ohio": "OH", "Oklahoma": "OK", "Oregon": "OR",
    "Pennsylvania": "PA", "Rhode Island": "RI", "South Carolina": "SC",
    "South Dakota": "SD", "Tennessee": "TN", "Texas": "TX", "Utah": "UT",
    "Vermont": "VT", "Virginia": "VA", "Washington": "WA",
    "West Virginia": "WV", "Wisconsin": "WI", "Wyoming": "WY",
    "District of Columbia": "DC",
}

# Suffixes to strip from Census place names
PLACE_SUFFIXES = re.compile(
    r"\s+(city|town|village|borough|municipality|CDP|"
    r"city and borough|consolidated government|"
    r"metro government|metropolitan government|"
    r"urban county government|unified government|"
    r"city \(balance\))$",
    re.IGNORECASE,
)


def normalize_census_name(raw: str) -> tuple[str, str] | None:
    """Parse Census NAME field like 'Boston city, Massachusetts' -> ('Boston', 'MA')."""
    if "," not in raw:
        return None
    parts = raw.rsplit(",", 1)
    place = parts[0].strip()
    state_full = parts[1].strip()

    state_code = STATE_CODES.get(state_full)
    if not state_code:
        return None

    # Strip place type suffixes
    place = PLACE_SUFFIXES.sub("", place).strip()

    return place, state_code


def find_pop_column(headers: list[str]) -> str | None:
    """Find the most recent POPESTIMATE column."""
    pop_cols = [h for h in headers if h.startswith("POPESTIMATE")]
    if not pop_cols:
        return None
    return sorted(pop_cols)[-1]  # Most recent year


def main():
    parser = argparse.ArgumentParser(description="Merge Census population into cities JSON")
    parser.add_argument("--cities", required=True, help="Path to cities JSON file")
    parser.add_argument("--census", required=True, help="Path to Census population CSV")
    parser.add_argument("--output", required=True, help="Output path for enriched JSON")
    parser.add_argument("--pop-column", help="Specific population column name (default: most recent POPESTIMATE*)")
    parser.add_argument("--dry-run", action="store_true", help="Print stats without writing")
    args = parser.parse_args()

    # Load cities
    with open(args.cities) as f:
        cities = json.load(f)
    print(f"Loaded {len(cities)} cities from {args.cities}")

    # Load Census data
    census = {}
    with open(args.census, newline="", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        pop_col = args.pop_column or find_pop_column(reader.fieldnames)
        if not pop_col:
            print(f"ERROR: No POPESTIMATE column found. Available: {reader.fieldnames}", file=sys.stderr)
            sys.exit(1)
        print(f"Using population column: {pop_col}")

        for row in reader:
            parsed = normalize_census_name(row.get("NAME", ""))
            if not parsed:
                continue
            name, state = parsed
            try:
                pop = int(row[pop_col])
            except (ValueError, KeyError):
                continue
            # Use (name_lower, state) as key; keep highest pop if duplicates
            key = (name.lower(), state)
            if key not in census or pop > census[key][1]:
                census[key] = (name, pop)

    print(f"Parsed {len(census)} Census places")

    # Merge
    matched = 0
    unmatched_cities = []
    for city in cities:
        key = (city["name"].lower(), city["state"])
        if key in census:
            city["population"] = census[key][1]
            matched += 1
        else:
            if city.get("population", 0) == 0:
                unmatched_cities.append(f"{city['name']}, {city['state']}")

    print(f"\nResults:")
    print(f"  Matched: {matched}/{len(cities)} ({100*matched/len(cities):.1f}%)")
    print(f"  Unmatched (pop=0): {len(unmatched_cities)}")

    if len(unmatched_cities) <= 20:
        for name in unmatched_cities:
            print(f"    - {name}")

    if args.dry_run:
        print("\nDry run — no file written.")
        return

    # Write output
    with open(args.output, "w") as f:
        json.dump(cities, f, separators=(",", ":"))
    print(f"\nWrote {args.output}")


if __name__ == "__main__":
    main()
