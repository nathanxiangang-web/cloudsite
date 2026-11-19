package cloudpath

import "strings"

func IsPrivate(resourcePath string) bool {
	return strings.HasSuffix(resourcePath, "/_users") || strings.Contains(resourcePath, "/_users/")
}
