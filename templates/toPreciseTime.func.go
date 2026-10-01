//go:build ignore
package templates

import "time"
import "fmt"

func(s interface{}) string {
	timeObj, ok := s.(time.Time)
	if !ok {
		timestamp, ok := s.(int64)
		if !ok {
			panic("Cannot determine time type")
		}
		timeObj = time.Unix(timestamp, 0)
		s := timeObj.Format(time.DateTime)
		zone, _ := timeObj.Zone()
		return fmt.Sprintf("%s %s", s, zone)
	} else {
		s := timeObj.Format(time.DateTime)
		zone, _ := timeObj.Zone()
		return fmt.Sprintf("%s %s", s, zone)
	}
}

