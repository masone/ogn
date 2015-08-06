package startlist

import (
	"fmt"
	"github.com/kellydunn/golang-geo"
	"github.com/masone/ogn/startlist_db"
	"os"
	"strconv"
	"time"
)

var (
	home_point             *geo.Point
	home_elevation         float64
	elevation_threshold    float64 = 20  // in meters
	distance_threshold     float64 = 2   // in kilometers
	winch_launch_threshold float64 = 500 // in meters
)

func Init() {
	home_lat, _ := strconv.ParseFloat(os.Getenv("AF_LAT"), 64)
	home_lng, _ := strconv.ParseFloat(os.Getenv("AF_LNG"), 64)
	home_point = geo.NewPoint(home_lat, home_lng)
	home_elevation, _ = strconv.ParseFloat(os.Getenv("AF_ELEVATION"), 64)

	startlist_db.Init()
}

func ProcessEntry(t time.Time, id string, cs string, lat float64, lon float64, alt float64, climb_rate float64) {
	if climb_rate != 0.0 {
		startlist_db.InsertClimbRate(t, id, cs, climb_rate)
	}
	startlist_db.InsertAltitude(t, id, cs, alt)

	//plane := geo.NewPoint(lat, lon)
	//fmt.Printf("    %s - %fkm away - %fm\n", cs, home_point.GreatCircleDistance(plane), alt)

	if near_coordinates(lat, lon) && near_altitude(alt) {
		handleOnGround(t, id, cs)
	} else {
		handleAirborne(t, id, cs)
	}
}

func handleOnGround(t time.Time, id string, cs string) {
	lastPosition := startlist_db.GetLastPosition(id, t)
	startlist_db.InsertPosition(t, id, cs, "gnd")

	if lastPosition == "air" {
		fmt.Printf("*** %s landed %s at %s\n", cs, t, id)
		startlist_db.InsertLanding(t, id, cs)
	} else {
		//fmt.Printf("    %s still on ground %s\n", cs, id)
	}
}

func handleAirborne(t time.Time, id string, cs string) {
	lastPosition := startlist_db.GetLastPosition(id, t)
	startlist_db.InsertPosition(t, id, cs, "air")

	if lastPosition == "gnd" {
		launch_type := detectLaunchType(id, t)

		fmt.Printf("*** %s started (%s) at %s, %s\n", cs, launch_type, t, id)
		startlist_db.InsertStart(t, id, cs, launch_type)
	} else {
		//fmt.Printf("    %s still airborne %s\n", cs, id)
	}
}

func detectLaunchType(id string, t time.Time) string {
	max := startlist_db.GetRecentMaxAlt(id, t)
	if (max - home_elevation) > winch_launch_threshold {
		return "W"
	} else {
		return "A"
	}
	// else if no towplane in sight
}

func near_coordinates(lat float64, lon float64) bool {
	plane := geo.NewPoint(lat, lon)
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
