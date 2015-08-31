package startlist_db

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"os"
	"time"
)

type Flight struct {
	Id               uint   `gorm:"primary_key"`
	OgnId            string `validate:"presence"`
	Callsign         string `sql:"size(12)"`
	LaunchType       string `sql:"size(1)"`
	Start            int64
	FormattedStart   string
	Landing          int64
	FormattedLanding string
	Duration         int64
	TowFlight        int64 `sql:"references flights(id)"`
}
type Position struct {
	Id            uint  `gorm:"primary_key"`
	Time          int64 `validate:"presence"`
	FormattedTime string
	OgnId         string `validate:"presence"`
	Callsign      string `sql:"size(12)"`
	Position      string `validate:"presence" sql:"size(3)"`
	ClimbRate     float64
	Altitude      float64 `validate:"presence"`
	Lat           float64 `validate:"presence"`
	Lon           float64 `validate:"presence"`
}

var db gorm.DB

func Init() {
	var err error
	db, err = gorm.Open("postgres", os.Getenv("DATABASE_URL"))
	checkErr(err)

	db.DropTable(&Flight{})
	db.CreateTable(&Flight{})
	db.DropTable(&Position{})
	db.CreateTable(&Position{})
	fmt.Println("")
}

func InsertStart(t time.Time, id string, cs string) uint {
	flight := initializeFlight(id, cs)
	flight.Start = t.Unix()
	flight.FormattedStart = t.String()

	db.Save(&flight)
	return flight.Id
}

func InsertLanding(t time.Time, id string, cs string) {
	var flight Flight
	var results []Flight
	db.Where("ogn_id = ? AND landing = 0", id).Last(&results)

	if len(results) > 0 {
		flight = results[0]
	} else {
		flight = initializeFlight(id, cs)
	}

	flight.Landing = t.Unix()
	flight.FormattedLanding = t.String()
	flight.Duration = flight.Landing - flight.Start

	if flight.Callsign != " HB-KDF (  )" {
		printFlight(flight)
	}

	query := db.Save(&flight)
	checkErr(query.Error)
}

func InsertPosition(t time.Time, id string, cs string, pos string, cr float64, alt float64, lat float64, lon float64) {
	position := &Position{
		OgnId:         id,
		Callsign:      cs,
		Time:          t.Unix(),
		FormattedTime: t.String(),
		Position:      pos,
		ClimbRate:     cr,
		Altitude:      alt,
		Lat:           lat,
		Lon:           lon,
	}
	query := db.Save(position)
	checkErr(query.Error)
}

func UpdateFlightDetails(id string, t time.Time, lt string, tfId int64) {
	var flight Flight
	query := db.Where("ogn_id = ? AND start = ?", id, t.Unix()).Last(&flight)
	checkErr(query.Error)

	if lt != "" {
		flight.LaunchType = lt
	}
	if tfId > 0 {
		flight.TowFlight = tfId
	}
	//fmt.Printf("update flight %s %s %d\n", id, lt, tfId)

	db.Save(&flight)
}

func GetLastPosition(id string, t time.Time) (p string) {
	var results []Position

	past := t.Add(-5 * time.Minute)
	query := db.
		Where("ogn_id = ? AND time < ? AND time > ? AND position != ?", id, t.Unix(), past.Unix(), "").
		Last(&results)

	checkErr(query.Error)
	if len(results) > 0 {
		return results[0].Position
	} else {
		return ""
	}
}

func GetRecentMaxAlt(id string, t time.Time) float64 {
	var results []Position

	past := t.Add(-30 * time.Second)
	future := t.Add(30 * time.Second)

	query := db.
		Select("MAX(altitude) as altitude").
		Where("ogn_id = ? AND time < ? AND time > ?", id, future.Unix(), past.Unix()).
		Find(&results)

	checkErr(query.Error)
	if len(results) > 0 {
		return results[0].Altitude
	} else {
		return 0.0
	}
}

func GetRecentParallelStart(id string, t time.Time) string {
	var results []Flight

	past := t.Add(-30 * time.Second)
	future := t.Add(30 * time.Second)
	query := db.
		Select("ogn_id").
		Where("ogn_id != ? AND start < ? AND start > ?", id, future.Unix(), past.Unix()).
		Last(&results)

	checkErr(query.Error)
	if len(results) > 0 {
		return results[0].OgnId
	} else {
		return ""
	}
}

func GetRecentAvgAltitude(id string, t time.Time) float64 {
	var results []Position

	past := t.Add(-30 * time.Second)
	query := db.
		Select("AVG(altitude) as altitude").
		Where("ogn_id = ? AND time < ? AND time > ?", id, t.Unix(), past.Unix()).
		Group("id").
		Last(&results)

	checkErr(query.Error)
	if len(results) > 0 {
		return results[0].Altitude
	} else {
		return 0.0
	}
}

func printFlight(f Flight) {
	duration, _ := time.ParseDuration(fmt.Sprintf("%ds", f.Duration))

	fmt.Printf("%s | %s | %02d:%02d | %02d:%02d | %d:%02d \n",
		f.Callsign,
		f.LaunchType,
		time.Unix(f.Start, 0).Hour(),
		time.Unix(f.Start, 0).Minute(),
		time.Unix(f.Landing, 0).Hour(),
		time.Unix(f.Landing, 0).Minute(),
		int(duration.Hours()),
		int(duration.Minutes()),
	)
}

func initializeFlight(id string, cs string) Flight {
	return Flight{
		OgnId:    id,
		Callsign: cs,
	}
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
