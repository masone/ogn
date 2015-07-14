package aprs

import (
	"bufio"
	"fmt"
	"github.com/martinhpedersen/libfap-go"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

func Listen(processor func(packet *fap.Packet)) {
	defer fap.Cleanup()

	connection := connect()
	authenticate(connection)
	keepalive(connection)
	each_message(connection, processor)
}

func connect() net.Conn {
	connection, err := net.Dial("tcp", "aprs.glidernet.org:14580")
	if err != nil {
		panic(err)
	} else {
		return connection
	}
}

func authenticate(c net.Conn) {
	fmt.Fprintf(c, "user SGW134975 pass -1 vers libfapGo 0.0.1 filter r/46.8333/8.3333/200\n")
}

func keepalive(c net.Conn) {
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		for t := range ticker.C {
			fmt.Fprintf(c, "# SGW keepalive %s\n", t)
		}
	}()
}

func each_message(c net.Conn, processor func(packet *fap.Packet)) {
	reader := bufio.NewReader(c)
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
