package adminops

import "strconv"

func validCapacity(c Capacity) bool {
	switch c.Status {
	case "AUTHENTICATED":
		total, e := strconv.ParseInt(c.TotalBytes, 10, 64)
		used, e2 := strconv.ParseInt(c.UsedBytes, 10, 64)
		remaining, e3 := strconv.ParseInt(c.RemainingBytes, 10, 64)
		return e == nil && e2 == nil && e3 == nil && total > 0 && used >= 0 && remaining >= 0 && used <= total && remaining == total-used && !c.ProbedAt.IsZero()
	case "NOT_AUTHENTICATED", "NOT_CONNECTED", "UNKNOWN":
		return c.TotalBytes == "" && c.UsedBytes == "" && c.RemainingBytes == ""
	}
	return false
}
