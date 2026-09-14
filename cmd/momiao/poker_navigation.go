package main

import "regexp"

var pokerTableNavigation = regexp.MustCompile(`^/poker/table/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func pokerBrowserRoute(path string) bool {
	return path == "/poker" || pokerTableNavigation.MatchString(path)
}
