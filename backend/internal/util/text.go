package util

import (
	"regexp"
	"strings"
)

var thinkTagRegex = regexp.MustCompile(`(?is)<think>.*?</think>`)

func NormalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func StripThinking(s string) string {
	return thinkTagRegex.ReplaceAllString(s, " ")
}

func ClampWords(s string, maxWords int) string {
	parts := strings.Fields(s)
	if len(parts) <= maxWords {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts[:maxWords], " ")
}
