package util

import (
	"testing"
	"time"

	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
)

func TestEasterSunday(t *testing.T) {
	for year, want := range map[int]civil.Date{
		2026: civil.DateFor(2026, time.April, 5),
		2027: civil.DateFor(2027, time.March, 28),
		2028: civil.DateFor(2028, time.April, 16),
		2029: civil.DateFor(2029, time.April, 1),
		2030: civil.DateFor(2030, time.April, 21),
		// the two exceptions of the formula
		1981: civil.DateFor(1981, time.April, 19),
		1954: civil.DateFor(1954, time.April, 18),
	} {
		assert.Equal(t, want, EasterSunday(year), "Easter %d", year)
	}
}

func TestIsAustrianHoliday(t *testing.T) {
	holidays := []civil.Date{
		civil.DateFor(2026, 1, 1), civil.DateFor(2026, 1, 6), civil.DateFor(2026, 4, 6), // Easter Monday
		civil.DateFor(2026, 5, 1), civil.DateFor(2026, 5, 14), // Ascension
		civil.DateFor(2026, 5, 25), civil.DateFor(2026, 6, 4), // Whit Monday, Corpus Christi
		civil.DateFor(2026, 8, 15), civil.DateFor(2026, 10, 26), civil.DateFor(2026, 11, 1),
		civil.DateFor(2026, 12, 8), civil.DateFor(2026, 12, 25), civil.DateFor(2026, 12, 26),
		civil.DateFor(2027, 3, 29), civil.DateFor(2027, 5, 6), civil.DateFor(2027, 5, 17), civil.DateFor(2027, 5, 27),
	}
	for _, d := range holidays {
		assert.True(t, IsAustrianHoliday(d), "%s is a holiday", d)
	}
	for _, d := range []civil.Date{
		civil.DateFor(2026, 12, 24), civil.DateFor(2026, 12, 31), // working days
		civil.DateFor(2026, 4, 5), civil.DateFor(2026, 4, 3), // Easter Sunday (a Sunday anyway), Good Friday
	} {
		assert.False(t, IsAustrianHoliday(d), "%s is no holiday", d)
	}
	assert.True(t, IsWorkingDay(civil.DateFor(2026, 12, 24)))
	assert.True(t, IsWorkingDay(civil.DateFor(2026, 12, 31)))
	assert.False(t, IsWorkingDay(civil.DateFor(2026, 11, 28)), "Saturday")
	assert.False(t, IsWorkingDay(civil.DateFor(2026, 11, 29)), "Sunday")
}

func TestRegistrationWindow(t *testing.T) {
	// acceptance criterion of platform#272: on 26.11.2026 the 13.01.2027 is the 30th working day
	from, to := RegistrationWindow(civil.DateFor(2026, 11, 26))
	assert.Equal(t, civil.DateFor(2026, 11, 27), from)
	assert.Equal(t, civil.DateFor(2027, 1, 13), to)
	assert.True(t, civil.DateFor(2027, 1, 14).After(to), "14.01.2027 is beyond the window")

	// a holiday right after today does not count; the window starts tomorrow all the same
	from, to = RegistrationWindow(civil.DateFor(2026, 12, 7))
	assert.Equal(t, civil.DateFor(2026, 12, 8), from)
	assert.Equal(t, civil.DateFor(2027, 1, 22), to)
}

func TestDayInVienna(t *testing.T) {
	// 23:30 UTC on 26.11. is already 27.11. in Vienna (UTC+1)
	assert.Equal(t, civil.DateFor(2026, 11, 27), DayInVienna(time.Date(2026, 11, 26, 23, 30, 0, 0, time.UTC)))
	// summer time (UTC+2)
	assert.Equal(t, civil.DateFor(2026, 7, 1), DayInVienna(time.Date(2026, 6, 30, 22, 30, 0, 0, time.UTC)))
	assert.Equal(t, civil.DateFor(2026, 6, 30), DayInVienna(time.Date(2026, 6, 30, 21, 30, 0, 0, time.UTC)))
}

func TestRegistrationStart(t *testing.T) {
	today := civil.DateFor(2026, 11, 26)
	member := func(d civil.Date) civil.NullDate { return civil.NullDate{Date: d, Valid: true} }
	tests := []struct {
		name       string
		registered civil.Date
		since      civil.NullDate
		want       civil.Date
	}{
		{"requested date later than member since", civil.DateFor(2027, 1, 1), member(civil.DateFor(2026, 12, 1)), civil.DateFor(2027, 1, 1)},
		{"member since later than requested date", civil.DateFor(2026, 12, 1), member(civil.DateFor(2027, 1, 1)), civil.DateFor(2027, 1, 1)},
		{"tie", civil.DateFor(2026, 12, 3), member(civil.DateFor(2026, 12, 3)), civil.DateFor(2026, 12, 3)},
		{"both in the past: tomorrow", civil.DateFor(2026, 1, 1), member(civil.DateFor(2025, 1, 1)), civil.DateFor(2026, 11, 27)},
		{"today: tomorrow", today, member(civil.DateFor(2025, 1, 1)), civil.DateFor(2026, 11, 27)},
		{"no member since", civil.DateFor(2026, 12, 10), civil.NullDate{}, civil.DateFor(2026, 12, 10)},
		{"nothing set: tomorrow", civil.Date{}, civil.NullDate{}, civil.DateFor(2026, 11, 27)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RegistrationStart(tt.registered, tt.since, today))
		})
	}
}
