package secret

import (
	"strings"
	"unicode/utf8"
)

// Mask conceals short values and reveals only the edges of longer values.
func Mask(value string) string {
	length := utf8.RuneCountInString(value)
	if length == 0 {
		return ""
	}
	if length <= 8 {
		return strings.Repeat("*", length)
	}

	var firstEnd, lastStart int
	runeIndex := 0
	for byteIndex := range value {
		if runeIndex == 4 {
			firstEnd = byteIndex
		}
		if runeIndex == length-4 {
			lastStart = byteIndex
			break
		}
		runeIndex++
	}
	return value[:firstEnd] + "…" + value[lastStart:]
}
