package flarm

import (
	"regexp"
	"strconv"
	"strings"
)

type Comment struct {
	Id             string
	SignalStrength string
	Frequency      string
	Rot            string
	ClimbRate      float64
	Fpm            string
	Errors         string
}

// example comment: id02DF0A52 -019fpm +0.0rot 55.2dB 0e -9.9kHz gps3x6
func ParseComment(c string) Comment {
	items := strings.Split(strings.TrimSpace(c), " ")

	comment := Comment{
		Id:             extractId(items[0]),
		Fpm:            items[1],
		Rot:            items[2],
		ClimbRate:      extractClimbRate(items[2]),
		SignalStrength: items[3],
		Errors:         items[4],
		Frequency:      items[5]}

	return comment
}

func extractId(s string) string {
	id_matcher := regexp.MustCompile(`id\w{2}(\w+)`)
	return strings.TrimSpace(id_matcher.FindStringSubmatch(s)[1])
}

func extractClimbRate(s string) float64 {
	climb_rate_matcher := regexp.MustCompile(`([+-]\d+\.\d+)rot`)
	climb_rate_str := climb_rate_matcher.FindStringSubmatch(s)[1]
	climb_rate, _ := strconv.ParseFloat(climb_rate_str, 64)
	return climb_rate
}
