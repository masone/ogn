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
	distance_threshold     float64 = 0.5 // in kilometers
	winch_launch_threshold float64 = 200 // in meters
	tow_threshold          float64 = 20  // in meters
)

func Init() {
	home_lat, _ := strconv.ParseFloat(os.Getenv("AF_LAT"), 64)
	home_lng, _ := strconv.ParseFloat(os.Getenv("AF_LNG"), 64)
	home_point = geo.NewPoint(home_lat, home_lng)
	home_elevation, _ = strconv.ParseFloat(os.Getenv("AF_ELEVATION"), 64)

	startlist_db.Init()
	fmt.Println("")
}

func ProcessEntry(ft time.Time, id string, cs string, lat float64, lon float64, alt float64, climb_rate float64) {
	//plane := geo.NewPoint(lat, lon)
	//fmt.Printf("    %s - %fkm away - %fm\n", cs, home_point.GreatCircleDistance(plane), alt)
	t := packetTime(ft)
	var pos string
	nc := near_coordinates(lat, lon)
	ng := near_ground(alt)

	if nc && ng {
		pos = "gnd"
		handleOnGround(t, id, cs)
	} else if !nc && !ng {
		pos = "air"
		handleAirborne(t, id, cs)
	} else {
		// Position is not 100% clear. Store, but don't qualify.
		// Prevents detecting false starts/landings.
		// The Flarm altitude is sometimes off (eg. when the device boots up).
		pos = ""
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
		//fmt.Printf("*** %s started (%s) at %s\n", cs, t, id)
		startlist_db.InsertStart(t, id, cs)

		go func() {
			delay := 20 * time.Second
			delayed := t.Add(delay)
			time.Sleep(delay)
			detectLaunchType(id, t, delayed, cs)
		}() // TODO: sync
	} else {
		//fmt.Printf("    %s still airborne %s\n", cs, id)
	}
}

func detectLaunchType(id string, t time.Time, dt time.Time, cs string) string {
	max := startlist_db.GetRecentMaxAlt(id, t)
	diff := math.Abs(max - home_elevation)

	var lt string
	if detectTow(id, t, dt, cs) {
		lt = "A"
	} else if diff > winch_launch_threshold {
		//fmt.Printf("    %s started W (%s), height gain %f\n", cs, t, diff)
		lt = "W"
	} else {
		//fmt.Printf("    %s started S (%s), height gain %f\n", cs, t, diff)
		lt = "S"
	}

	startlist_db.UpdateFlightDetails(id, t, lt, 0)
	return lt
}

func detectTow(id string, t time.Time, dt time.Time, cs string) bool {
	last_id := startlist_db.GetRecentParallelStart(id, dt)
	if last_id != "" {
		alts1 := startlist_db.GetRecentAvgAltitude(id, dt)
		alts2 := startlist_db.GetRecentAvgAltitude(last_id, dt)

		diff := math.Abs(alts2 - alts1)

		//fmt.Printf("    %s started in parallel with %s - h diff %f\n", id, last_id, diff)
		if diff < tow_threshold {
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

func near_ground(a float64) bool {
	if a > home_elevation-elevation_threshold && a < home_elevation+elevation_threshold {
		return true
	} else {
		return false
	}
}

// The Flarm timestamp uses a Hours/Minutes/Seconds format. The date is not passed explicitely.
// libfap-go messes up when converting this to a time, resulting in the correct time for different dates.
func packetTime(t time.Time) time.Time {
	now := time.Now()
	hour, min, sec := t.Clock()
	day := now.Day()
	month := now.Month()
	year := now.Year()

	return time.Date(year, month, day, hour, min, sec, 0, time.Local)
}
