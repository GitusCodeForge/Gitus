package gitlib

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"
)

type AuthorTime struct {
	AuthorName string
	AuthorEmail string
	Time time.Time
}

func parseTimezoneOffset(s string) (int, error) {
	// returns seconds east of UTC.
	if s == "Z" { return 0, nil }
	if len(s) != 5 { return 0, errors.New("Invalid timezone offset string") }
	hour := ((int(s[1]) - int('0')) * 10) + (int(s[2]) - int('0'))
	minute := ((int(s[3]) - int('0')) * 10) + (int(s[4]) - int('0'))
	total := (hour * 60 + minute) * 60
	if s[0] == '-' { total = -total }
	return total, nil
}

var reAuthorTime = regexp.MustCompile(`([^<>]+)\s*<([^>]*)>\s*([^\s]+)\s*([^\s]+)`)
func parseAuthorTime(s string) AuthorTime {
	res := AuthorTime {
		AuthorName: "",
		AuthorEmail: "",
		Time: time.Unix(0, 0),
	}
	re := reAuthorTime
	matchres := re.FindSubmatch([]byte(s))
	if len(matchres) <= 0 {
		log.Fatalf("Cannot parse author-time: %s\n", s)
	}
	res.AuthorName = string(matchres[1])
	res.AuthorEmail = string(matchres[2])
	timeStampString := string(matchres[3])
	timezoneOffsetString := string(matchres[4])
	timeStampInt, err := strconv.ParseInt(timeStampString, 10, 64)
	if err != nil { timeStampInt = 0 }
	timePiece := time.Unix(timeStampInt, 0).UTC()
	timezoneOffsetInt, err := parseTimezoneOffset(timezoneOffsetString)
	if err != nil { log.Fatal(err) }
	timezone := time.FixedZone("UTC" + timezoneOffsetString, timezoneOffsetInt)
	res.Time = timePiece.In(timezone)
	return res
}


func (at *AuthorTime) String() string {
	_, i := at.Time.Zone()
	// i is "seconds east of UTC"...
	positive := i > 0
	if i < 0 { i = -i }
	totalMinutes := i / 60
	hours := totalMinutes / 60
	remainderMinutes := totalMinutes % 60
	var offsetString string
	if positive {
		offsetString = fmt.Sprintf("+%02d%02d", hours, remainderMinutes)
	} else {
		offsetString = fmt.Sprintf("-%02d%02d", hours, remainderMinutes)
	}
	return fmt.Sprintf("%s <%s> %d %s", at.AuthorName, at.AuthorEmail, at.Time.Unix(), offsetString)
}

