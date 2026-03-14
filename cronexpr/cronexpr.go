package cronexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Expression represents a parsed 5-field cron expression:
// minute hour day-of-month month day-of-week.
type Expression struct {
	minute field
	hour   field
	dom    field
	month  field
	dow    field
}

type field struct {
	values []bool
	any    bool
	min    int
	max    int
}

// Parse validates and parses a standard 5-field cron expression.
func Parse(spec string) (Expression, error) {
	parts := strings.Fields(strings.TrimSpace(spec))
	if len(parts) != 5 {
		return Expression{}, fmt.Errorf("invalid cron schedule %q: expected 5 fields", spec)
	}

	minute, err := parseField(parts[0], 0, 59, false)
	if err != nil {
		return Expression{}, fmt.Errorf("invalid minute field: %w", err)
	}
	hour, err := parseField(parts[1], 0, 23, false)
	if err != nil {
		return Expression{}, fmt.Errorf("invalid hour field: %w", err)
	}
	dom, err := parseField(parts[2], 1, 31, false)
	if err != nil {
		return Expression{}, fmt.Errorf("invalid day-of-month field: %w", err)
	}
	month, err := parseField(parts[3], 1, 12, false)
	if err != nil {
		return Expression{}, fmt.Errorf("invalid month field: %w", err)
	}
	dow, err := parseField(parts[4], 0, 6, true)
	if err != nil {
		return Expression{}, fmt.Errorf("invalid day-of-week field: %w", err)
	}

	return Expression{
		minute: minute,
		hour:   hour,
		dom:    dom,
		month:  month,
		dow:    dow,
	}, nil
}

// Next returns the next matching time strictly after after.
func (e Expression) Next(after time.Time) (time.Time, bool) {
	start := after.In(after.Location()).Truncate(time.Minute).Add(time.Minute)
	return e.scan(start, 10*366*24*60, 1)
}

// Prev returns the most recent matching time less than or equal to atOrBefore.
func (e Expression) Prev(atOrBefore time.Time) (time.Time, bool) {
	start := atOrBefore.In(atOrBefore.Location()).Truncate(time.Minute)
	return e.scan(start, 10*366*24*60, -1)
}

func (e Expression) scan(start time.Time, maxSteps int, dir int) (time.Time, bool) {
	current := start
	step := time.Minute
	if dir < 0 {
		step = -time.Minute
	}
	for i := 0; i < maxSteps; i++ {
		if e.matches(current) {
			return current, true
		}
		current = current.Add(step)
	}
	return time.Time{}, false
}

func (e Expression) matches(t time.Time) bool {
	month := int(t.Month())
	if !e.month.contains(month) {
		return false
	}
	hour := t.Hour()
	if !e.hour.contains(hour) {
		return false
	}
	minute := t.Minute()
	if !e.minute.contains(minute) {
		return false
	}

	domMatch := e.dom.contains(t.Day())
	dowMatch := e.dow.contains(int(t.Weekday()))

	switch {
	case e.dom.any && e.dow.any:
		return true
	case e.dom.any:
		return dowMatch
	case e.dow.any:
		return domMatch
	default:
		return domMatch || dowMatch
	}
}

func (f field) contains(v int) bool {
	if v < f.min || v > f.max {
		return false
	}
	return f.values[v-f.min]
}

func parseField(spec string, min, max int, sundayAlias bool) (field, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return field{}, fmt.Errorf("empty field")
	}
	out := field{
		values: make([]bool, max-min+1),
		min:    min,
		max:    max,
	}

	segments := strings.Split(spec, ",")
	for _, rawSegment := range segments {
		segment := strings.TrimSpace(rawSegment)
		if segment == "" {
			return field{}, fmt.Errorf("invalid list segment")
		}
		if err := markSegment(&out, segment, sundayAlias); err != nil {
			return field{}, err
		}
	}
	out.any = allTrue(out.values)
	return out, nil
}

func markSegment(out *field, segment string, sundayAlias bool) error {
	base := segment
	step := 1

	if strings.Contains(segment, "/") {
		parts := strings.Split(segment, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid step segment %q", segment)
		}
		base = strings.TrimSpace(parts[0])
		if base == "" {
			return fmt.Errorf("invalid step base in %q", segment)
		}
		v, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || v <= 0 {
			return fmt.Errorf("invalid step value in %q", segment)
		}
		step = v
	}

	start := out.min
	end := out.max

	switch {
	case base == "*":
		// already [min, max]
	case strings.Contains(base, "-"):
		parts := strings.Split(base, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid range %q", base)
		}
		left, err := parseFieldValue(parts[0], out.min, out.max, sundayAlias)
		if err != nil {
			return err
		}
		right, err := parseFieldValue(parts[1], out.min, out.max, sundayAlias)
		if err != nil {
			return err
		}
		if right < left {
			return fmt.Errorf("invalid range %q", base)
		}
		start = left
		end = right
	default:
		value, err := parseFieldValue(base, out.min, out.max, sundayAlias)
		if err != nil {
			return err
		}
		start = value
		if strings.Contains(segment, "/") {
			end = out.max
		} else {
			end = value
		}
	}

	for value := start; value <= end; value += step {
		out.values[value-out.min] = true
	}
	return nil
}

func parseFieldValue(raw string, min, max int, sundayAlias bool) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", raw)
	}
	if sundayAlias && value == 7 {
		value = 0
	}
	if value < min || value > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", value, min, max)
	}
	return value, nil
}

func allTrue(values []bool) bool {
	for _, value := range values {
		if !value {
			return false
		}
	}
	return len(values) > 0
}
