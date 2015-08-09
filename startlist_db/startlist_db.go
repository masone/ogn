package startlist_db

import (
	"encoding/json"
	"fmt"
	influxdb "github.com/influxdb/influxdb/client"
	"log"
	"net/url"
	"os"
	"time"
)

var connection *influxdb.Client

func Init() {
	u, err := url.Parse(
		fmt.Sprintf("http://%s:%s",
			os.Getenv("INFLUX_HOST"),
			os.Getenv("INFLUX_PORT"),
		))

	if err != nil {
		log.Fatal(err)
	}

	conf := influxdb.Config{
		URL:      *u,
		Username: os.Getenv("INFLUX_USERNAME"),
		Password: os.Getenv("INFLUX_PASSWORD"),
	}

	connection, err = influxdb.NewClient(conf)
	if err != nil {
		log.Fatal(err)
	}
}

func InsertLanding(t time.Time, id string, cs string) {
	point := influxdb.Point{
		Measurement: "landings",
		Tags: map[string]string{
			"id": id, // indexed
		},
		Fields: map[string]interface{}{
			"cs": cs, // not indexed
		},
		Time: t,
	}

	insertPoint(point)
}

func InsertStart(t time.Time, id string, cs string, launch_type string) {
	point := influxdb.Point{
		Measurement: "starts",
		Tags: map[string]string{
			"id": id, // indexed
		},
		Fields: map[string]interface{}{
			// not indexed
			"cs":          cs,
			"launch_type": launch_type,
		},
		Time: t,
	}

	insertPoint(point)
}

func InsertPosition(t time.Time, id string, cs string, pos string, cr float64, alt float64, lat float64, lon float64) {
	point := influxdb.Point{
		Measurement: "positions",
		Tags: map[string]string{
			// indexed
			"id":  id,
			"cs":  cs,
			"pos": pos,
		},
		Fields: map[string]interface{}{
			// not indexed
			"pos": pos,
			"cs":  cs,
			"cr":  cr,
			"alt": alt,
			"lat": lat,
			"lon": lon,
		},
		Time: t,
	}

	insertPoint(point)
}

func GetLastPosition(id string, t time.Time) (p string) {
	cmd := fmt.Sprintf(
		"SELECT LAST(pos) FROM %s WHERE id='%s' AND time < %ds AND time > %ds - 5m LIMIT 1",
		"positions",
		id,
		t.Unix(),
		t.Unix(),
	)

	q := influxdb.Query{
		Command:  cmd,
		Database: os.Getenv("INFLUX_DATABASE"),
	}

	if response, err := connection.Query(q); err == nil {
		if response.Error() != nil {
			log.Fatal(response.Error())
		} else {
			res := response.Results
			series := res[0].Series
			if len(series) != 0 && series[0].Values[0][1] != nil {
				return series[0].Values[0][1].(string)
			}
		}
	}
	return
}

func GetRecentMaxAlt(id string, t time.Time) (c float64) {
	cmd := fmt.Sprintf(
		"SELECT MAX(alt) FROM %s WHERE id='%s' AND time < %ds AND time > %ds - 30s",
		"positions",
		id,
		t.Unix(),
		t.Unix(),
	)

	q := influxdb.Query{
		Command:  cmd,
		Database: os.Getenv("INFLUX_DATABASE"),
	}

	if response, err := connection.Query(q); err == nil {
		if response.Error() != nil {
			log.Fatal(response.Error())
		} else {
			res := response.Results
			series := res[0].Series
			if len(series) != 0 {
				val, err := series[0].Values[0][1].(json.Number).Float64()
				if err != nil {
					log.Fatal(err)
				}

				return val
			}
		}
	}
	return 0.0
}

func GetRecentStarts(id string, t time.Time) (c float64) {
	cmd := fmt.Sprintf(
		"SELECT MAX(alt) FROM %s WHERE id='%s' AND time < %ds AND time > %ds - 30s",
		"positions",
		id,
		t.Unix(),
		t.Unix(),
	)

	q := influxdb.Query{
		Command:  cmd,
		Database: os.Getenv("INFLUX_DATABASE"),
	}

	if response, err := connection.Query(q); err == nil {
		if response.Error() != nil {
			log.Fatal(response.Error())
		} else {
			res := response.Results
			series := res[0].Series
			if len(series) != 0 {
				val, err := series[0].Values[0][1].(json.Number).Float64()
				if err != nil {
					log.Fatal(err)
				}

				return val
			}
		}
	}
	return 0.0
}

func insertPoint(p influxdb.Point) {
	pts := []influxdb.Point{p}
	bps := influxdb.BatchPoints{
		Points:          pts,
		Database:        os.Getenv("INFLUX_DATABASE"),
		RetentionPolicy: "default",
	}
	_, err := connection.Write(bps)
	if err != nil {
		log.Fatal(err)
	}
}
