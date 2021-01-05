package utils

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Common durations that is .
// There are some definitions for units of Day and larger .
const (
	Second = time.Second
	Minute = Second * 60
	Hour   = Minute * 60
	Day    = Hour * 24
	Week   = Day * 7
	Month  = Day * 30
	Year   = Day * 365

	TimeLayout = time.RFC3339Nano

	// RFC3339NanoFixed is our own version of RFC339Nano because we want one
	// that pads the nano seconds part with zeros to ensure
	// the timestamps are aligned in the logs.
	RFC3339NanoFixed = "2006-01-02T15:04:05.000000000Z07:00"
)

var errInvalid = errors.New("invalid time")

// FormatTimeInterval is used to show the time interval from input time to now.
func FormatTimeInterval(input int64) (formattedTime string, err error) {
	start := time.Unix(0, input)
	diff := time.Since(start)

	// That should not happen.
	if diff < 0 {
		return "", errInvalid
	}

	timeThresholds := []time.Duration{Year, Month, Week, Day, Hour, Minute, Second}
	timeNames := []string{"year", "month", "week", "day", "hour", "minute", "second"}

	for i, threshold := range timeThresholds {
		if diff >= threshold {
			count := int(diff / threshold)
			formattedTime += strconv.Itoa(count) + " " + timeNames[i]
			if count > 1 {
				formattedTime += "s"
			}
			break
		}
	}

	if diff < Second {
		formattedTime += "0 second"
	}

	return formattedTime, nil
}

// GetUnixTimestamp will parse the value into time and get the nano-timestamp
// in string.
//
// NOTE: if the value is not relative time, GetUnixTimestamp will use RFC3339
// format to parse the value.
func GetUnixTimestamp(value string, base time.Time) (string, error) {
	// time.ParseDuration will handle the 5h, 7d relative time.
	if d, err := time.ParseDuration(value); value != "0" && err == nil {
		return strconv.FormatInt(base.Add(-d).Unix(), 10), nil
	}

	var (
		// rfc3399
		layoutDate            = "2006-01-02"
		layoutDateWithH       = "2006-01-02T15"
		layoutDateWithHM      = "2006-01-02T15:04"
		layoutDateWithHMS     = "2006-01-02T15:04:05"
		layoutDateWithHMSNano = "2006-01-02T15:04:05.999999999"

		layout string
	)

	// if the value doesn't contain any z, Z, +, T, : and -, it maybe
	// timestamp and we should return value.
	if !strings.ContainsAny(value, "zZ+.:T-") {
		return value, nil
	}

	// if the value containns any z, Z or +, we should parse it with timezone
	isLocal := !(strings.ContainsAny(value, "zZ+") || strings.Count(value, "-") == 3)

	if strings.Contains(value, ".") {
		// if the value contains ., we should parse it with nano
		if isLocal {
			layout = layoutDateWithHMSNano
		} else {
			layout = layoutDateWithHMSNano + "Z07:00"
		}
	} else if strings.Contains(value, "T") {
		// if the value contains T, we should parse it with h:m:s
		numColons := strings.Count(value, ":")

		// NOTE:
		// from https://tools.ietf.org/html/rfc3339
		//
		// time-numoffset = ("+" / "-") time-hour [[":"] time-minute]
		//
		// if the value has zero with +/-, it may contains the extra
		// colon like +08:00, which we should remove the extra colon.
		if !isLocal && !strings.ContainsAny(value, "zZ") && numColons > 0 {
			numColons--
		}

		switch numColons {
		case 0:
			layout = layoutDateWithH
		case 1:
			layout = layoutDateWithHM
		default:
			layout = layoutDateWithHMS
		}

		if !isLocal {
			layout += "Z07:00"
		}
	} else if isLocal {
		layout = layoutDate
	} else {
		layout = layoutDate + "Z07:00"
	}

	var t time.Time
	var err error

	if isLocal {
		t, err = time.ParseInLocation(layout, value, time.FixedZone(base.Zone()))
	} else {
		t, err = time.Parse(layout, value)
	}

	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%09d", t.Unix(), int64(t.Nanosecond())), nil
}

// ParseTimestamp returns seconds and nanoseconds.
//
// 1. If the value is empty, it will return default second, the second arg.
// 2. If the incoming nanosecond portion is longer or shorter than 9 digits,
//	it will be converted into 9 digits nanoseconds.
func ParseTimestamp(value string, defaultSec int64) (int64, int64, error) {
	if value == "" {
		return defaultSec, 0, nil
	}

	vs := strings.SplitN(value, ".", 2)

	// for second
	s, err := strconv.ParseInt(vs[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	if len(vs) != 2 {
		return s, 0, nil
	}

	// for nanoseconds
	n, err := strconv.ParseInt(vs[1], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	// convert the n into 9 digits
	n = int64(float64(n) * math.Pow(float64(10), float64(9-len(vs[1]))))
	return s, n, nil
}
