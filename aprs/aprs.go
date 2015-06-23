package aprs

import (
	"bufio"
	"fmt"
	"github.com/martinhpedersen/libfap-go"
	"io"
	"log"
	"net"
	"strings"
)

func Listen(processor func(packet *fap.Packet)) {
	defer fap.Cleanup()
	connection := connect()
	each_message(connection, processor)
}

func connect() net.Conn {
	connection, err := net.Dial("tcp", "aprs.glidernet.org:14580")
	fmt.Fprintf(connection, "user SGW134975 pass -1 vers libfapGo 0.0.1 filter r/46.8333/8.3333/200\n")
	if err != nil {
		panic(err)
	} else {
		return connection
	}
}

func each_message(connection net.Conn, processor func(packet *fap.Packet)) {
	reader := bufio.NewReader(connection)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			log.Fatal(err)
		} else if err != nil {
			panic(err)
		}

		// APRS,qAS: aircraft beacon
		// APRS,TCPIP*,qAC: ground station beacon
		if strings.Contains(line, ">APRS,qAS") {
			packet, err := fap.ParseAprs(line, false)
			if err == nil {
				processor(packet)
			}
		}
	}
}
