package flarm

import (
	"regexp"
	"strings"
)

type Comment struct {
	Id             string
	SignalStrength string
	Frequency      string
	Rotation       string
	Fpm            string
	Errors         string
	Gps            string
}

// example comment: id02DF0A52 -019fpm +0.0rot 55.2dB 0e -9.9kHz gps3x6
func ParseComment(c string) Comment {
	items := strings.Split(strings.TrimSpace(c), " ")

	id_matcher := regexp.MustCompile(`id\w{2}(\w+)`)
	id := strings.TrimSpace(id_matcher.FindStringSubmatch(items[0])[1])

	comment := Comment{
		Id:             id,
		Fpm:            items[1],
		Rotation:       items[2],
		SignalStrength: items[3],
		Errors:         items[4],
		Frequency:      items[5],
		Gps:            items[6]}

	return comment
}
