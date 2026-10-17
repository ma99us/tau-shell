package main

import (
	"net/url"
	"strings"
)

func isBundledAppNavigation(target string) bool {
	parsed, err := url.Parse(target)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	return strings.EqualFold(parsed.Host, "wails.localhost") ||
		strings.EqualFold(parsed.Host, "wails.localhost:80")
}
