package main

import (
	"fmt"
	"github.com/martinhpedersen/libfap-go"
	"github.com/masone/ogn/aprs"
	"github.com/masone/ogn/ddb"
	"github.com/masone/ogn/flarm"
)

type Beacon struct {
	*fap.Packet
	ddb.Aircraft
	flarm.Comment
}

func main() {
	ddb.Download()
	aprs.Listen(process_message)
}

// packet: https://github.com/martinhpedersen/libfap-go/blob/832c8336185c0a6de6b792ad1531a30eac09398d/packet.go
func process_message(p *fap.Packet) {
	c := flarm.ParseComment(p.Comment)
	a, ok := ddb.GetAircraft(c.Id)
	var b Beacon

	if ok {
		b = Beacon{Packet: p, Comment: c, Aircraft: a}
	} else {
		b = Beacon{Packet: p, Comment: c}
	}

	fmt.Printf("%+v", b)
	//fmt.Printf("%+v", b)
}
