package startlist_db

import (
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

func InsertLanding(id string, cs string) {
	point := influxdb.Point{
		Measurement: "landings",
		Tags: map[string]string{
			"id": id, // indexed
		},
		Fields: map[string]interface{}{
			"cs": cs, // not indexed
		},
		Time: time.Now(),
	}

	insertPoint(point)
}

func InsertStart(id string, cs string) {
	point := influxdb.Point{
		Measurement: "starts",
		Tags: map[string]string{
			"id": id, // indexed
		},
		Fields: map[string]interface{}{
			"cs": cs, // not indexed
		},
		Time: time.Now(),
	}

	insertPoint(point)
}

func InsertPosition(id string, cs string, p string) {
	point := influxdb.Point{
		Measurement: "positions",
		Tags: map[string]string{
			"id":  id, // indexed
			"pos": p,  // indexed
		},
		Fields: map[string]interface{}{
			"cs":  cs, // not indexed
			"pos": p,
		},
		Time: time.Now(),
	}
	insertPoint(point)
}

func GetLastPosition(id string) (p string) {
	cmd := fmt.Sprintf(
		"SELECT LAST(pos) FROM %s WHERE id='%s' time > now() - 5m LIMIT 1",
		"positions",
		id,
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
				return series[0].Values[0][1].(string)
			}
		}
	}
	return
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
