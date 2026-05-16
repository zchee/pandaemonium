// Copyright 2026 The pandaemonium Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package toml

import (
	"fmt"
	"time"
)

type dateTimeKind uint8

const (
	dateTimeKindInvalid dateTimeKind = iota
	dateTimeKindOffset
	dateTimeKindLocalDateTime
	dateTimeKindLocalDate
	dateTimeKindLocalTime
)

func parseDateTimeValue(raw []byte) (any, dateTimeKind, error) {
	if t, _, err := parseOffsetDateTime(raw); err == nil {
		return t, dateTimeKindOffset, nil
	}
	if dt, err := parseLocalDateTime(raw); err == nil {
		return dt, dateTimeKindLocalDateTime, nil
	}
	if d, err := parseLocalDate(raw); err == nil {
		return d, dateTimeKindLocalDate, nil
	}
	if lt, err := parseLocalTime(raw); err == nil {
		return lt, dateTimeKindLocalTime, nil
	}
	return nil, dateTimeKindInvalid, fmt.Errorf("toml: invalid datetime %q", raw)
}

func parseDateTimeAsTime(raw []byte, span [2]int, opts ...Option) (time.Time, error) {
	v, kind, err := parseDateTimeValue(raw)
	if err != nil {
		return time.Time{}, err
	}
	if kind == dateTimeKindOffset {
		return v.(time.Time), nil
	}

	cfg := &Decoder{}
	for _, opt := range opts {
		opt(cfg)
	}
	if !cfg.localAsUTC {
		return time.Time{}, &LocalTimeIntoTimeError{Kind: TokenKindValueDatetime, Source: append([]byte(nil), raw...), Span: span}
	}

	switch x := v.(type) {
	case LocalDateTime:
		return time.Date(x.Year, time.Month(x.Month), x.Day, x.Hour, x.Minute, x.Second, x.Nanosecond, time.UTC), nil
	case LocalDate:
		return time.Date(x.Year, time.Month(x.Month), x.Day, 0, 0, 0, 0, time.UTC), nil
	case LocalTime:
		return time.Date(0, time.January, 1, x.Hour, x.Minute, x.Second, x.Nanosecond, time.UTC), nil
	default:
		return time.Time{}, fmt.Errorf("toml: unsupported datetime value %T", v)
	}
}

func parseOffsetDateTime(raw []byte) (time.Time, int8, error) {
	date, timeStart, err := parseDatePrefix(raw)
	if err != nil {
		return time.Time{}, 0, err
	}
	if timeStart >= len(raw) || !isDateTimeSeparator(raw[timeStart]) {
		return time.Time{}, 0, fmt.Errorf("toml: missing datetime separator")
	}
	clock, next, err := parseTimePrefix(raw[timeStart+1:])
	if err != nil {
		return time.Time{}, 0, err
	}
	off := timeStart + 1 + next
	if off >= len(raw) {
		return time.Time{}, 0, fmt.Errorf("toml: missing datetime offset")
	}
	loc, err := parseZone(raw[off:])
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Date(date.Year, time.Month(date.Month), date.Day, clock.Hour, clock.Minute, clock.Second, clock.Nanosecond, loc), clock.nanoDigits, nil
}

func parseLocalDateTime(raw []byte) (LocalDateTime, error) {
	date, timeStart, err := parseDatePrefix(raw)
	if err != nil {
		return LocalDateTime{}, err
	}
	if timeStart >= len(raw) || !isDateTimeSeparator(raw[timeStart]) {
		return LocalDateTime{}, fmt.Errorf("toml: missing datetime separator")
	}
	clock, next, err := parseTimePrefix(raw[timeStart+1:])
	if err != nil {
		return LocalDateTime{}, err
	}
	if timeStart+1+next != len(raw) {
		return LocalDateTime{}, fmt.Errorf("toml: unexpected trailing datetime data")
	}
	return LocalDateTime{
		Year:       date.Year,
		Month:      date.Month,
		Day:        date.Day,
		Hour:       clock.Hour,
		Minute:     clock.Minute,
		Second:     clock.Second,
		Nanosecond: clock.Nanosecond,
		nanoDigits: clock.nanoDigits,
	}, nil
}

func parseLocalDate(raw []byte) (LocalDate, error) {
	date, next, err := parseDatePrefix(raw)
	if err != nil {
		return LocalDate{}, err
	}
	if next != len(raw) {
		return LocalDate{}, fmt.Errorf("toml: unexpected trailing date data")
	}
	return date, nil
}

func parseLocalTime(raw []byte) (LocalTime, error) {
	clock, next, err := parseTimePrefix(raw)
	if err != nil {
		return LocalTime{}, err
	}
	if next != len(raw) {
		return LocalTime{}, fmt.Errorf("toml: unexpected trailing time data")
	}
	return clock, nil
}

func parseDatePrefix(raw []byte) (LocalDate, int, error) {
	if len(raw) < len("0000-00-00") {
		return LocalDate{}, 0, fmt.Errorf("toml: short date")
	}
	if raw[4] != '-' || raw[7] != '-' {
		return LocalDate{}, 0, fmt.Errorf("toml: malformed date separators")
	}
	year, ok := parseFixedDigits(raw[0:4])
	if !ok {
		return LocalDate{}, 0, fmt.Errorf("toml: malformed year")
	}
	month, ok := parseFixedDigits(raw[5:7])
	if !ok {
		return LocalDate{}, 0, fmt.Errorf("toml: malformed month")
	}
	day, ok := parseFixedDigits(raw[8:10])
	if !ok {
		return LocalDate{}, 0, fmt.Errorf("toml: malformed day")
	}
	if err := validateDate(year, month, day); err != nil {
		return LocalDate{}, 0, err
	}
	return LocalDate{Year: year, Month: month, Day: day}, len("0000-00-00"), nil
}

func parseTimePrefix(raw []byte) (LocalTime, int, error) {
	if len(raw) < len("00:00:00") {
		return LocalTime{}, 0, fmt.Errorf("toml: short time")
	}
	if raw[2] != ':' || raw[5] != ':' {
		return LocalTime{}, 0, fmt.Errorf("toml: malformed time separators")
	}
	hour, ok := parseFixedDigits(raw[0:2])
	if !ok {
		return LocalTime{}, 0, fmt.Errorf("toml: malformed hour")
	}
	minute, ok := parseFixedDigits(raw[3:5])
	if !ok {
		return LocalTime{}, 0, fmt.Errorf("toml: malformed minute")
	}
	second, ok := parseFixedDigits(raw[6:8])
	if !ok {
		return LocalTime{}, 0, fmt.Errorf("toml: malformed second")
	}
	next := len("00:00:00")
	nanos := 0
	nanoDigits := int8(0)
	if next < len(raw) && raw[next] == '.' {
		next++
		fracStart := next
		for next < len(raw) && raw[next] >= '0' && raw[next] <= '9' {
			next++
		}
		if next == fracStart {
			return LocalTime{}, 0, fmt.Errorf("toml: empty fractional second")
		}
		if next-fracStart > 9 {
			return LocalTime{}, 0, fmt.Errorf("toml: fractional second precision exceeds nanoseconds")
		}
		nanoDigits = int8(next - fracStart)
		for i := fracStart; i < next; i++ {
			nanos = nanos*10 + int(raw[i]-'0')
		}
		for range 9 - int(nanoDigits) {
			nanos *= 10
		}
	}
	if err := validateLocalClock(hour, minute, second, nanos, nanoDigits); err != nil {
		return LocalTime{}, 0, err
	}
	return LocalTime{Hour: hour, Minute: minute, Second: second, Nanosecond: nanos, nanoDigits: nanoDigits}, next, nil
}

func parseZone(raw []byte) (*time.Location, error) {
	if len(raw) == 1 && raw[0] == 'Z' {
		return time.UTC, nil
	}
	if len(raw) != len("+00:00") || (raw[0] != '+' && raw[0] != '-') || raw[3] != ':' {
		return nil, fmt.Errorf("toml: malformed datetime offset")
	}
	hour, ok := parseFixedDigits(raw[1:3])
	if !ok {
		return nil, fmt.Errorf("toml: malformed offset hour")
	}
	minute, ok := parseFixedDigits(raw[4:6])
	if !ok {
		return nil, fmt.Errorf("toml: malformed offset minute")
	}
	if hour > 23 || minute > 59 {
		return nil, fmt.Errorf("toml: offset out of range")
	}
	seconds := hour*3600 + minute*60
	if raw[0] == '-' {
		seconds = -seconds
	}
	return time.FixedZone(string(raw), seconds), nil
}

func parseFixedDigits(raw []byte) (int, bool) {
	v := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		v = v*10 + int(ch-'0')
	}
	return v, true
}

func validateDate(year, month, day int) error {
	if year < 0 || year > 9999 {
		return fmt.Errorf("toml: year out of range")
	}
	if month < 1 || month > 12 {
		return fmt.Errorf("toml: month out of range")
	}
	if day < 1 || day > daysInMonth(year, month) {
		return fmt.Errorf("toml: day out of range")
	}
	return nil
}

func validateLocalClock(hour, minute, second, nanos int, nanoDigits int8) error {
	if hour < 0 || hour > 23 {
		return fmt.Errorf("toml: hour out of range")
	}
	if minute < 0 || minute > 59 {
		return fmt.Errorf("toml: minute out of range")
	}
	if second < 0 || second > 59 {
		return fmt.Errorf("toml: second out of range")
	}
	if nanos < 0 || nanos >= int(time.Second) {
		return fmt.Errorf("toml: nanosecond out of range")
	}
	if nanoDigits < 0 || nanoDigits > 9 {
		return fmt.Errorf("toml: fractional precision out of range")
	}
	if nanoDigits == 0 && nanos != 0 {
		return fmt.Errorf("toml: nanosecond requires fractional precision")
	}
	return nil
}

func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeapYear(year) {
			return 29
		}
		return 28
	default:
		return 0
	}
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func isDateTimeSeparator(ch byte) bool {
	return ch == 'T' || ch == 't' || ch == ' '
}
