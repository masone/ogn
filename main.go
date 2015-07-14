package main

import (
	"fmt"
	"github.com/martinhpedersen/libfap-go"
	"github.com/masone/ogn/aprs"
	"github.com/masone/ogn/ddb"
	"github.com/masone/ogn/flarm"
	"github.com/masone/ogn/startlist"
)

type Beacon struct {
	*fap.Packet
	ddb.Aircraft
	flarm.Comment
}

func main() {
	ddb.Download()
	startlist.Init()
	aprs.Listen(process_message)
}

// packet: https://github.com/martinhpedersen/libfap-go/blob/832c8336185c0a6de6b792ad1531a30eac09398d/packet.go
func process_message(p *fap.Packet) {
	c := flarm.ParseComment(p.Comment)
	a, ok := ddb.GetAircraft(c.Id)
	var b Beacon

	if ok {
		b = Beacon{Packet: p, Comment: c, Aircraft: a}
		if b.Comment.Id != "" && a.Callsign != "" {
			startlist.ProcessEntry(b.Comment.Id, b.Aircraft.Callsign, b.Packet.Latitude, b.Packet.Longitude, b.Packet.Altitude)
		}
	} else {
		b = Beacon{Packet: p, Comment: c}
	}

	//fmt.Printf("%+v", b)
}

func (b Beacon) String() string {
	return fmt.Sprintf("%s (%s/%s) @%f,%f %fm\n", b.Comment.Id, b.Aircraft.Callsign, b.Aircraft.Registration, b.Packet.Latitude, b.Packet.Longitude, b.Altitude)
}
