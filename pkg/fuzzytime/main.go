package fuzzytime

import (
	"fmt"
	"time"
)

// a simple library for fuzzy time, e.g. the things that are like
// "1 day ago" "last week" "3 years ago".
// Lord, i have to consider i18n for this as well...

func TimeSpanToFuzzyTimeString(s time.Duration) string {
	minute := int64(s.Minutes())
	hour := int64(s.Hours())
	if minute < 1 { return "just now" }
	if hour < 1 { return fmt.Sprintf("%d minutes ago", minute) }
	if minute < 100 { return "an hour ago" }
	if hour < 10 { return fmt.Sprintf("%d hours ago", hour) }
	if hour < 14 { return "half a day ago" }
	if hour < 20 { return fmt.Sprintf("%d hours ago", hour) }
	if hour < 26 { return "a day ago" }
	var dayCount int64 = hour / 24
	var dayRemain int64 = hour % 24
	var weekCount int64 = dayCount / 7
	var weekRemain int64 = dayCount % 7
	var monthCount int64 = dayCount / 30
	var monthRemain int64 = dayCount % 30
	var yearCount int64 = monthCount / 12
	var yearRemain int64 = monthCount % 12
	if yearCount < 1 {
		if monthCount < 1 {
			if monthRemain < 25 {
				if weekCount < 1 {
					if weekRemain < 6 {
						day := dayCount
						if dayRemain >= 20 { day += 1 }
						return fmt.Sprintf("%d days ago", day)
					} else { return "a week ago" }
				} else {
					week := weekCount
					if weekRemain >= 6 { week += 1 }
					return fmt.Sprintf("%d weeks ago", week)
				}
			} else { return "a month ago" }
		} else {
			month := monthCount
			if monthRemain >= 25 { month += 1 }
			return fmt.Sprintf("%d months ago", month)
		}
	} else {
		if yearCount == 1 {
			if yearRemain == 0 {
				return "1 year ago"
			} else if  yearRemain == 1 {
				return "1 year 1 month ago"
			} else {
				return fmt.Sprintf("1 year %d months ago", yearRemain)
			}
		} else {
			if yearRemain == 0 {
				return fmt.Sprintf("%d years ago", yearCount)
			} else if yearRemain == 1 {
				return fmt.Sprintf("%d years 1 month ago", yearCount)
			} else {
				return fmt.Sprintf("%d years %d months ago", yearCount, yearRemain)
			}
		}
	}
}



