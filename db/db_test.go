package db

import (
	"testing"
	"time"
)

func TestNextAlarm(t *testing.T) {
	tz, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		alarm    time.Time
		dt       time.Time
		offset   time.Duration
		expected time.Time
	}{
		{
			name:     "middle_after",
			alarm:    time.Date(2021, 9, 28, 15, 0, 0, 0, time.Local),
			dt:       time.Date(2021, 10, 2, 15, 18, 19, 20, time.Local),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 10, 5, 15, 0, 0, 0, time.Local),
		},
		{
			name:     "middle_before",
			alarm:    time.Date(2021, 9, 28, 15, 0, 0, 0, time.Local),
			dt:       time.Date(2021, 10, 2, 14, 18, 19, 20, time.Local),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 10, 5, 15, 0, 0, 0, time.Local),
		},
		{
			name:     "border_before",
			alarm:    time.Date(2021, 9, 28, 15, 0, 0, 0, time.Local),
			dt:       time.Date(2021, 10, 5, 14, 18, 19, 20, time.Local),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 10, 5, 15, 0, 0, 0, time.Local),
		},
		{
			name:     "border_after",
			alarm:    time.Date(2021, 9, 28, 15, 0, 0, 0, time.Local),
			dt:       time.Date(2021, 10, 5, 15, 18, 19, 20, time.Local),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 10, 12, 15, 0, 0, 0, time.Local),
		},
		{
			name:     "equal",
			alarm:    time.Date(2021, 9, 28, 15, 0, 0, 0, time.Local),
			dt:       time.Date(2021, 10, 5, 15, 0, 0, 0, time.Local),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 10, 5, 15, 0, 0, 0, time.Local),
		},
		{
			name:     "big_offset",
			alarm:    time.Date(2021, 9, 28, 15, 0, 0, 0, time.Local),
			dt:       time.Date(2021, 10, 5, 23, 12, 13, 0, time.Local),
			offset:   30 * 24 * time.Hour,
			expected: time.Date(2021, 10, 28, 15, 0, 0, 0, time.Local),
		},
		{
			name:     "too_early",
			alarm:    time.Date(1980, 7, 29, 15, 0, 0, 0, time.Local),
			dt:       time.Date(2021, 10, 5, 23, 12, 13, 0, time.Local),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 10, 12, 15, 0, 0, 0, time.Local),
		},
		{
			name:     "another_tz",
			alarm:    time.Date(2021, 1, 5, 15, 0, 0, 0, tz),
			dt:       time.Date(2021, 7, 7, 23, 12, 13, 0, tz),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 7, 13, 15, 0, 0, 0, tz),
		},
		{
			name:     "another_tz_revert",
			alarm:    time.Date(2021, 7, 13, 15, 0, 0, 0, tz),
			dt:       time.Date(2021, 12, 8, 23, 12, 13, 0, tz),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 12, 14, 15, 0, 0, 0, tz),
		},
		{
			name:     "big_another_tz",
			alarm:    time.Date(1984, 1, 3, 15, 0, 0, 0, tz),
			dt:       time.Date(2021, 7, 7, 23, 12, 13, 0, tz),
			offset:   7 * 24 * time.Hour,
			expected: time.Date(2021, 7, 13, 15, 0, 0, 0, tz),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(tt *testing.T) {
			a := nextAlarm(c.alarm, c.dt, c.offset)
			if !a.Equal(c.expected) {
				tt.Errorf("failed compare %v != %v", c.expected, a)
			}
		})
	}
}
