package fileaccess

import (
	"fmt"
	"strconv"
	"strings"
)

func FormatRange(start, end int64) string { return fmt.Sprintf("bytes=%d-%d", start, end) }
func ByteRange(value string, size int64) (int64, int64, bool, error) {
	if value == "" {
		return 0, size - 1, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") || size <= 0 || len(value) > 100 {
		return 0, 0, false, ErrRange
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, false, ErrRange
	}
	parse := func(s string) (int64, error) {
		if s == "" || strings.Trim(s, "0123456789") != "" {
			return 0, ErrRange
		}
		return strconv.ParseInt(s, 10, 64)
	}
	var start, end int64
	var err error
	if parts[0] == "" {
		suffix, e := parse(parts[1])
		if e != nil || suffix <= 0 {
			return 0, 0, false, ErrRange
		}
		if suffix > size {
			suffix = size
		}
		start = size - suffix
		end = size - 1
	} else {
		start, err = parse(parts[0])
		if err != nil || start >= size {
			return 0, 0, false, ErrRange
		}
		end = size - 1
		if parts[1] != "" {
			end, err = parse(parts[1])
			if err != nil || end < start {
				return 0, 0, false, ErrRange
			}
			if end >= size {
				end = size - 1
			}
		}
	}
	return start, end, true, nil
}
