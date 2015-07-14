package startlist

import (
	"fmt"
	"github.com/kellydunn/golang-geo"
	"github.com/masone/ogn/startlist_db"
)

func Init() {
	startlist_db.Init()
}

func ProcessEntry(id string, cs string, lat float64, lon float64, alt float64) {
	if near_coordinates(lat, lon) && near_altitude(alt) {
		handleOnGround(id, cs)
	} else {
		handleAirborne(id, cs)
	}
}

func handleOnGround(id string, cs string) {
	lastPosition := startlist_db.GetLastPosition(id)
	startlist_db.InsertPosition(id, cs, "gnd")

	if lastPosition == "air" {
		fmt.Printf("%s **** just landed\n", cs)
		startlist_db.InsertLanding(id, cs)
	} else {
		fmt.Printf("%s still on ground\n", cs)
	}
}

func handleAirborne(id string, cs string) {
	lastPosition := startlist_db.GetLastPosition(id)
	startlist_db.InsertPosition(id, cs, "air")

	if lastPosition == "gnd" {
		fmt.Printf("%s **** just started\n", cs)
		startlist_db.InsertStart(id, cs)
	} else {
		fmt.Printf("%s still airborne %s\n", cs, id)
	}
}

func near_coordinates(lat float64, lng float64) bool {
	home := geo.NewPoint(47.5147283, 8.7722307)
	plane := geo.NewPoint(lat, lng)
	var threshold float64 = 1 // in kilometers

	if home.GreatCircleDistance(plane) <= threshold {
		return true
	} else {
		return false
	}
}

func near_altitude(a float64) bool {
	var home float64 = 470
	var threshold float64 = 20

	if a > home-threshold && a < home+threshold {
		return true
	} else {
		return false
	}
}
