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

func ProcessEntry(id string, cs string, lat float64, lon float64, alt float64, climb_rate float64) {
	if climb_rate != 0.0 {
		startlist_db.InsertClimbRate(id, cs, climb_rate)
	}
	startlist_db.InsertAltitude(id, cs, alt)

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
		fmt.Printf("*** %s landed %s\n", cs, id)
		startlist_db.InsertLanding(id, cs)
	} else {
		//fmt.Printf("%s still on ground %s\n", cs, id)
	}
}

func handleAirborne(id string, cs string) {
	lastPosition := startlist_db.GetLastPosition(id)
	startlist_db.InsertPosition(id, cs, "air")

	if lastPosition == "gnd" {
		launch_type := detectLaunchType(id)

		fmt.Printf("*** %s started (%s), %s\n", cs, launch_type, id)
		startlist_db.InsertStart(id, cs, launch_type)
	} else {
		//fmt.Printf("%s still airborne %s\n", cs, id)
	}
}

func detectLaunchType(id string) string {
	climb := startlist_db.GetAverageClimb(id)
	if climb > 500 {
		return "W"
	} else {
		return "A"
	}
	// else if no towplane in sight
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
