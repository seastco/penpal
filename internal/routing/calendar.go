package routing

import (
	"math"
	"math/rand"
	"time"

	"github.com/seastco/penpal/internal/models"
)

// --- Timezone (cached) ---

var (
	locPhoenix, _    = time.LoadLocation("America/Phoenix")
	locHonolulu, _   = time.LoadLocation("Pacific/Honolulu")
	locAnchorage, _  = time.LoadLocation("America/Anchorage")
	locLosAngeles, _ = time.LoadLocation("America/Los_Angeles")
	locDenver, _     = time.LoadLocation("America/Denver")
	locChicago, _    = time.LoadLocation("America/Chicago")
	locNewYork, _    = time.LoadLocation("America/New_York")
)

// TimezoneForState returns a *time.Location for a US state code.
// Uses longitude as a tiebreaker for states that span two zones.
func TimezoneForState(state string, lng float64) *time.Location {
	switch state {
	case "AZ":
		return locPhoenix
	case "HI":
		return locHonolulu
	case "AK":
		return locAnchorage
	}
	return timezoneFromLng(lng)
}

// timezoneFromLng derives timezone from longitude bands.
func timezoneFromLng(lng float64) *time.Location {
	switch {
	case lng < -114.5:
		return locLosAngeles
	case lng < -100.5:
		return locDenver
	case lng < -84.5:
		return locChicago
	default:
		return locNewYork
	}
}

// --- Federal Holidays ---

// IsFederalHoliday returns true if the given date is an observed US federal holiday.
// Handles both fixed-date holidays (with Sat→Fri/Sun→Mon observed rules) and
// nth-weekday holidays (MLK Day, etc.).
func IsFederalHoliday(t time.Time) bool {
	_, m, d := t.Date()
	y := t.Year()
	wd := t.Weekday()

	switch m {
	case time.January:
		// New Year's Day (Jan 1)
		if isObservedFixed(y, time.January, 1, t) {
			return true
		}
		// MLK Day: 3rd Monday in January
		if wd == time.Monday && nthWeekday(y, time.January, time.Monday, 3) == d {
			return true
		}
	case time.February:
		// Presidents' Day: 3rd Monday in February
		if wd == time.Monday && nthWeekday(y, time.February, time.Monday, 3) == d {
			return true
		}
	case time.May:
		// Memorial Day: last Monday in May
		if wd == time.Monday && lastWeekday(y, time.May, time.Monday) == d {
			return true
		}
	case time.June:
		// Juneteenth (Jun 19)
		if isObservedFixed(y, time.June, 19, t) {
			return true
		}
	case time.July:
		// Independence Day (Jul 4)
		if isObservedFixed(y, time.July, 4, t) {
			return true
		}
	case time.September:
		// Labor Day: 1st Monday in September
		if wd == time.Monday && nthWeekday(y, time.September, time.Monday, 1) == d {
			return true
		}
	case time.October:
		// Columbus Day: 2nd Monday in October
		if wd == time.Monday && nthWeekday(y, time.October, time.Monday, 2) == d {
			return true
		}
	case time.November:
		// Veterans Day (Nov 11)
		if isObservedFixed(y, time.November, 11, t) {
			return true
		}
		// Thanksgiving: 4th Thursday in November
		if wd == time.Thursday && nthWeekday(y, time.November, time.Thursday, 4) == d {
			return true
		}
	case time.December:
		// Christmas (Dec 25)
		if isObservedFixed(y, time.December, 25, t) {
			return true
		}
		// New Year's observed: if Jan 1 is Saturday, Dec 31 Friday is observed
		jan1wd := time.Date(y+1, time.January, 1, 0, 0, 0, 0, t.Location()).Weekday()
		if d == 31 && jan1wd == time.Saturday && wd == time.Friday {
			return true
		}
	}

	return false
}

// isObservedFixed checks if date t is the observed date for a fixed holiday.
func isObservedFixed(year int, month time.Month, day int, t time.Time) bool {
	_, m, d := t.Date()
	if m != month {
		return false
	}

	holidayWd := time.Date(year, month, day, 0, 0, 0, 0, t.Location()).Weekday()

	switch holidayWd {
	case time.Saturday:
		return d == day-1 && t.Weekday() == time.Friday
	case time.Sunday:
		return d == day+1 && t.Weekday() == time.Monday
	default:
		return d == day
	}
}

// nthWeekday returns the day-of-month of the nth occurrence of wd in the given month.
func nthWeekday(year int, month time.Month, wd time.Weekday, n int) int {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := int(wd-first.Weekday()+7) % 7
	return 1 + offset + (n-1)*7
}

// lastWeekday returns the day-of-month of the last occurrence of wd in the given month.
func lastWeekday(year int, month time.Month, wd time.Weekday) int {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	for last.Weekday() != wd {
		last = last.AddDate(0, 0, -1)
	}
	return last.Day()
}

// --- Business Day Logic ---

// IsBusinessDay returns true if the given date is a mail processing day.
// First Class / Priority: Mon-Sat, no federal holidays.
// Express: every day except Christmas and New Year's.
func IsBusinessDay(t time.Time, express bool) bool {
	if express {
		_, m, d := t.Date()
		if (m == time.December && d == 25) || (m == time.January && d == 1) {
			return false
		}
		return true
	}
	if t.Weekday() == time.Sunday {
		return false
	}
	return !IsFederalHoliday(t)
}

// advanceToBusinessDay moves t forward until it lands on a business day.
func advanceToBusinessDay(t time.Time, express bool) time.Time {
	for !IsBusinessDay(t, express) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

const (
	postOfficeCutoff = 17 // 5PM local time
	processingStart  = 6  // 6AM local time
)

// NextProcessingStart returns when mail dropped at sendTime would start processing.
// If before the cutoff on a business day, processing starts immediately.
// Otherwise, it starts at 6AM on the next business day.
func NextProcessingStart(sendTime time.Time, loc *time.Location, express bool) time.Time {
	local := sendTime.In(loc)

	if local.Hour() < postOfficeCutoff && IsBusinessDay(local, express) {
		return sendTime
	}

	next := time.Date(local.Year(), local.Month(), local.Day(), processingStart, 0, 0, 0, loc)
	if local.Hour() >= postOfficeCutoff {
		next = next.AddDate(0, 0, 1)
	}
	return advanceToBusinessDay(next, express)
}

// Facility operating hours
const (
	facilityOpenStd   = 6  // 6AM for First Class / Priority
	facilityCloseStd  = 22 // 10PM
	facilityOpenExpr  = 5  // 5AM for Express
	facilityCloseExpr = 23 // 11PM
	deliveryStartStd  = 9  // 9AM
	deliveryEndStd    = 17 // 5PM
	deliveryStartExpr = 9  // 9AM
	deliveryEndExpr   = 19 // 7PM
)

// nextBusinessOpen returns the next facility opening time on a business day.
func nextBusinessOpen(t time.Time, loc *time.Location, openHr int, express bool) time.Time {
	local := t.In(loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), openHr, 0, 0, 0, loc)
	next = next.AddDate(0, 0, 1)
	return advanceToBusinessDay(next, express)
}

// SnapToFacilityHours adjusts a time to fall within facility operating hours.
// If outside hours, advances to the next facility open time.
func SnapToFacilityHours(t time.Time, loc *time.Location, express bool) time.Time {
	local := t.In(loc)
	openHr, closeHr := facilityOpenStd, facilityCloseStd
	if express {
		openHr, closeHr = facilityOpenExpr, facilityCloseExpr
	}

	hour := local.Hour()

	if hour >= openHr && hour < closeHr && IsBusinessDay(local, express) {
		return t
	}

	if hour >= closeHr || !IsBusinessDay(local, express) {
		return nextBusinessOpen(t, loc, openHr, express)
	}

	// Before opening on a business day: wait until open
	return time.Date(local.Year(), local.Month(), local.Day(), openHr, 0, 0, 0, loc)
}

// AddFacilityHours adds hours of processing time, skipping non-operating hours.
// If processing would extend past facility close, it continues the next business morning.
func AddFacilityHours(start time.Time, hours float64, loc *time.Location, express bool) time.Time {
	openHr, closeHr := facilityOpenStd, facilityCloseStd
	if express {
		openHr, closeHr = facilityOpenExpr, facilityCloseExpr
	}

	cursor := SnapToFacilityHours(start, loc, express)
	remaining := hours

	for remaining > 0 {
		local := cursor.In(loc)
		hoursLeftToday := float64(closeHr) - (float64(local.Hour()) + float64(local.Minute())/60.0)
		if hoursLeftToday <= 0 {
			cursor = nextBusinessOpen(cursor, loc, openHr, express)
			continue
		}

		if remaining <= hoursLeftToday {
			cursor = cursor.Add(time.Duration(remaining * float64(time.Hour)))
			remaining = 0
		} else {
			remaining -= hoursLeftToday
			cursor = nextBusinessOpen(cursor, loc, openHr, express)
		}
	}

	return cursor
}

// NextDeliverySlot returns a random time within the next delivery window.
// First Class/Priority: 9AM-5PM Mon-Sat. Express: 9AM-7PM every day.
func NextDeliverySlot(readyTime time.Time, loc *time.Location, express bool, rng *rand.Rand) time.Time {
	startHr, endHr := deliveryStartStd, deliveryEndStd
	if express {
		startHr, endHr = deliveryStartExpr, deliveryEndExpr
	}

	local := readyTime.In(loc)

	// If ready within today's delivery window on a business day, deliver today
	if local.Hour() >= startHr && local.Hour() < endHr && IsBusinessDay(local, express) {
		remainingMinutes := (endHr-local.Hour())*60 - local.Minute()
		if remainingMinutes > 0 {
			offset := rng.Intn(remainingMinutes)
			return local.Add(time.Duration(offset) * time.Minute)
		}
	}

	// Find next business day with a delivery window
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if local.Hour() >= endHr || !IsBusinessDay(local, express) {
		day = day.AddDate(0, 0, 1)
	}
	day = advanceToBusinessDay(day, express)

	// Random time within delivery window
	windowMinutes := (endHr - startHr) * 60
	offset := rng.Intn(windowMinutes)
	return time.Date(day.Year(), day.Month(), day.Day(), startHr, 0, 0, 0, loc).Add(time.Duration(offset) * time.Minute)
}

// --- Zone-Based Estimates ---

// DistanceToZone maps a haversine distance (miles) to a USPS zone (1-8).
func DistanceToZone(dist float64) int {
	switch {
	case dist <= 50:
		return 1
	case dist <= 150:
		return 2
	case dist <= 300:
		return 3
	case dist <= 600:
		return 4
	case dist <= 1000:
		return 5
	case dist <= 1400:
		return 6
	case dist <= 1800:
		return 7
	default:
		return 8
	}
}

// zoneDays maps [zone-1] → business days for each tier.
var zoneDaysFirstClass = [8]int{2, 2, 3, 3, 4, 4, 5, 5}
var zoneDaysPriority = [8]int{1, 2, 2, 2, 3, 3, 3, 3}
var zoneDaysExpress = [8]int{1, 1, 1, 1, 2, 2, 2, 2}

// EstimateBusinessDays returns the expected business-day transit time for a distance and tier.
func EstimateBusinessDays(dist float64, tier models.ShippingTier) int {
	zone := DistanceToZone(dist)
	idx := zone - 1
	switch tier {
	case models.TierFirstClass:
		return zoneDaysFirstClass[idx]
	case models.TierPriority:
		return zoneDaysPriority[idx]
	case models.TierExpress:
		return zoneDaysExpress[idx]
	default:
		return zoneDaysFirstClass[idx]
	}
}

// AddBusinessDays adds n business days to start, returning the resulting date.
func AddBusinessDays(start time.Time, days int, loc *time.Location, express bool) time.Time {
	t := start.In(loc)
	for days > 0 {
		t = t.AddDate(0, 0, 1)
		if IsBusinessDay(t, express) {
			days--
		}
	}
	return t
}

// EstimateDelivery computes the estimated delivery date for display.
func EstimateDelivery(dist float64, tier models.ShippingTier, sendTime time.Time, senderLoc *time.Location) time.Time {
	express := tier == models.TierExpress
	bizDays := EstimateBusinessDays(dist, tier)
	departure := NextProcessingStart(sendTime, senderLoc, express)
	delivery := AddBusinessDays(departure, bizDays, senderLoc, express)
	local := delivery.In(senderLoc)
	return time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, senderLoc)
}

// SampleDwellHours samples facility dwell time using a log-normal distribution.
// meanHours is the expected dwell time; sigma controls variance.
// Result is always positive and right-skewed.
func SampleDwellHours(meanHours, sigma float64, rng *rand.Rand) float64 {
	// Log-normal: if X ~ N(mu, sigma^2), then e^X ~ LogNormal
	// E[e^X] = e^(mu + sigma^2/2), so mu = ln(mean) - sigma^2/2
	mu := math.Log(meanHours) - sigma*sigma/2.0
	sample := math.Exp(mu + sigma*rng.NormFloat64())
	if sample < 1.0 {
		sample = 1.0
	}
	return sample
}
