package workbench

import (
	"strconv"
	"strings"
)

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func join(parts []string, sep string) string { return strings.Join(parts, sep) }

func itoa(n int) string { return strconv.Itoa(n) }
