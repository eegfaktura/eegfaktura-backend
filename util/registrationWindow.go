package util

import (
	"time"
	_ "time/tzdata" // Europe/Vienna also where the host has no zoneinfo

	"github.com/jjeffery/civil"
)

// RegistrationWorkingDays is the market rule for the start of a registration: at most the 30th working day after
// the day it is sent (platform#272).
const RegistrationWorkingDays = 30

var vienna, _ = time.LoadLocation("Europe/Vienna")

// DayInVienna is the calendar day of t in Austria.
func DayInVienna(t time.Time) civil.Date {
	return civil.DateOf(t.In(vienna))
}

// EasterSunday is Easter Sunday of the Gregorian calendar after Gauss's Easter formula, with the two exceptions
// (26 April becomes 19 April, 25 April becomes 18 April).
func EasterSunday(year int) civil.Date {
	a, b, c := year%19, year%4, year%7
	k := year / 100
	p := (13 + 8*k) / 25
	q := k / 4
	m := (15 - p + k - q) % 30
	n := (4 + k - q) % 7
	d := (19*a + m) % 30
	e := (2*b + 4*c + 6*d + n) % 7
	switch {
	case d == 29 && e == 6:
		return civil.DateFor(year, time.April, 19)
	case d == 28 && e == 6 && (11*m+11)%30 < 19:
		return civil.DateFor(year, time.April, 18)
	}
	// 22 + d + e March; DateFor turns e.g. 32 March into 1 April
	return civil.DateFor(year, time.March, 22+d+e)
}

// IsAustrianHoliday reports the statutory public holidays in Austria. 24 and 31 December are not among them.
func IsAustrianHoliday(day civil.Date) bool {
	switch day.Month() {
	case time.January:
		if day.Day() == 1 || day.Day() == 6 {
			return true
		}
	case time.May:
		if day.Day() == 1 {
			return true
		}
	case time.August:
		if day.Day() == 15 {
			return true
		}
	case time.October:
		if day.Day() == 26 {
			return true
		}
	case time.November:
		if day.Day() == 1 {
			return true
		}
	case time.December:
		if day.Day() == 8 || day.Day() == 25 || day.Day() == 26 {
			return true
		}
	}
	easter := EasterSunday(day.Year())
	// Easter Monday, Ascension Day, Whit Monday, Corpus Christi
	for _, offset := range []int{1, 39, 50, 60} {
		if day.Equal(easter.AddDate(0, 0, offset)) {
			return true
		}
	}
	return false
}

// IsWorkingDay is Monday to Friday without the Austrian public holidays.
func IsWorkingDay(day civil.Date) bool {
	wd := day.Weekday()
	return wd != time.Saturday && wd != time.Sunday && !IsAustrianHoliday(day)
}

// RegistrationWindow is the range of start dates a registration sent today may have: from tomorrow to the
// RegistrationWorkingDays-th working day after today, both included.
func RegistrationWindow(today civil.Date) (from, to civil.Date) {
	to = today
	for n := 0; n < RegistrationWorkingDays; {
		to = to.AddDate(0, 0, 1)
		if IsWorkingDay(to) {
			n++
		}
	}
	return today.AddDate(0, 0, 1), to
}

// RegistrationStart is the start date of a registration: the later of the requested date of the metering point
// (registeredSince) and "member since"; on a tie "member since". A day before tomorrow becomes tomorrow, as
// the registration always did.
func RegistrationStart(registeredSince civil.Date, participantSince civil.NullDate, today civil.Date) civil.Date {
	start := civil.Date{}
	if participantSince.Valid {
		start = participantSince.Date
	}
	if registeredSince.After(participantSince.Date) {
		start = registeredSince
	}
	if !start.After(today) {
		return today.AddDate(0, 0, 1)
	}
	return start
}
