package main

import (
	"fmt"
	"sort"
	"strings"
	"xabbo.b7c.io/goearth/shockwave/in"
)

type CountItem struct {
	Name  string
	Count int
	Class string
}

// Forked from 0xB0bba's G-Trader
func printCountResults(counts []CountItem) {
	// Sort the slice by count in descending order
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].Count > counts[j].Count
	})

	// Take top 30 (more doesn't fit in alert box)
	more := 0
	if len(counts) > 30 {
		more = len(counts) - 29
		counts = counts[:29]
	}
	var alert []string
	for _, countItem := range counts {
		alert = append(alert, fmt.Sprintf("%s %vx", countItem.Name, countItem.Count))
	}
	if more > 0 {
		alert = append(alert, fmt.Sprintf("... and %v more", more))
	}
	if len(alert) > 0 {
		ext.Send(in.SYSTEM_BROADCAST, []byte(strings.Join(alert, "\r")))
	}
}
