package startlist

import (
	"fmt"
	"github.com/kellydunn/golang-geo"
	"github.com/masone/ogn/startlist_db"
	"math"
	"os"
	"strconv"
	"time"
)

var (
	home_point             *geo.Point
	home_elevation         float64
	elevation_threshold    float64 = 20  // in meters
	distance_threshold     float64 = 2   // in kilometers
	winch_launch_threshold float64 = 400 // in meters
	tow_threshold          float64 = 20
)

func Init() {
	home_lat, _ := strconv.ParseFloat(os.Getenv("AF_LAT"), 64)
	home_lng, _ := strconv.ParseFloat(os.Getenv("AF_LNG"), 64)
	home_point = geo.NewPoint(home_lat, home_lng)
	home_elevation, _ = strconv.ParseFloat(os.Getenv("AF_ELEVATION"), 64)

	startlist_db.Init()
}

func ProcessEntry(t time.Time, id string, cs string, lat float64, lon float64, alt float64, climb_rate float64) {
	//plane := geo.NewPoint(lat, lon)
	//fmt.Printf("    %s - %fkm away - %fm\n", cs, home_point.GreatCircleDistance(plane), alt)

	var pos string
	if near_coordinates(lat, lon) && near_altitude(alt) {
		pos = "gnd"
		handleOnGround(t, id, cs)
	} else {
		pos = "air"
		handleAirborne(t, id, cs)
	}

	startlist_db.InsertPosition(t, id, cs, pos, climb_rate, alt, lat, lon)
}

func handleOnGround(t time.Time, id string, cs string) {
	lastPosition := startlist_db.GetLastPosition(id, t)

	if lastPosition == "air" {
		//fmt.Printf("*** %s landed %s at %s\n", cs, t, id)
		startlist_db.InsertLanding(t, id, cs)
	} else {
		//fmt.Printf("    %s still on ground %s\n", cs, id)
	}
}

func handleAirborne(t time.Time, id string, cs string) {
	lastPosition := startlist_db.GetLastPosition(id, t)

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
	diff := max - home_elevation
	if diff > winch_launch_threshold {
		return "W"
	} else if detectTow(id, t) {
		return "A"
	} else {
		return "S"
	}
}

func detectTow(id string, t time.Time) bool {
	last_id := startlist_db.GetRecentParallelStart(id, t)
	fmt.Println(last_id)
	if last_id != "" {
		alts1 := startlist_db.GetRecentAvgAltitude(id, t)
		alts2 := startlist_db.GetRecentAvgAltitude(last_id, t)

		diff := math.Abs(alts2 - alts1)

		fmt.Printf("    %s started in parallel with %s - h diff %f\n", id, last_id, diff)
		if diff < tow_threshold {
			fmt.Println("")
			return true
		}
	}
	return false
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
