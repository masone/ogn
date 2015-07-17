package startlist

import (
	"fmt"
	"github.com/kellydunn/golang-geo"
	"github.com/masone/ogn/startlist_db"
	"os"
	"strconv"
)

var (
	home_point          *geo.Point
	home_elevation      float64
	elevation_threshold float64 = 20 // in meters
	distance_threshold  float64 = 2  // in kilometers
)

func Init() {
	home_lat, _ := strconv.ParseFloat(os.Getenv("AF_LAT"), 64)
	home_lng, _ := strconv.ParseFloat(os.Getenv("AF_LNG"), 64)
	home_point = geo.NewPoint(home_lat, home_lng)
	home_elevation, _ = strconv.ParseFloat(os.Getenv("AF_ELEVATION"), 64)

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
	plane := geo.NewPoint(lat, lng)

	if home_point.GreatCircleDistance(plane) <= distance_threshold {
		return true
	} else {
		return false
	}
}

func near_altitude(a float64) bool {
	if a > home_elevation-elevation_threshold && a < home_elevation+elevation_threshold {
		return true
	} else {
		return false
	}
}
