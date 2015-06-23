package ddb

import (
	"encoding/csv"
	"log"
	"net/http"
	"strings"
)

type Aircraft struct {
	Id           string
	Model        string
	Registration string
	Callsign     string
}

type AircraftList map[string]Aircraft

var aircrafts AircraftList

func Download() {
	aircrafts = make(AircraftList)

	response, err := http.Get("http://ddb.glidernet.org/download")
	if err != nil {
		log.Fatal(err)
	}

	defer response.Body.Close()
	parse(response)
}

func parse(response *http.Response) {
	csv_reader := csv.NewReader(response.Body)
	csv, err := csv_reader.ReadAll()

	if err != nil {
		panic(err)
	}

	// TODO: iterator
	for _, line := range csv {
		a := Aircraft{
			Id:           strings.Replace(line[1], "'", "", 2),
			Model:        strings.Replace(line[2], "'", "", 2),
			Registration: strings.Replace(line[3], "'", "", 2),
			Callsign:     strings.Replace(line[4], "'", "", 2)}

		aircrafts[a.Id] = a
	}

}

func GetAircraft(id string) (Aircraft, bool) {
	a, ok := aircrafts[id]
	return a, ok
}
