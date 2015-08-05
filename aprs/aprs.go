package aprs

import (
	"bufio"
	"fmt"
	"github.com/martinhpedersen/libfap-go"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func Listen(processor func(packet *fap.Packet)) {
	defer fap.Cleanup()
	var reader *bufio.Reader

	if len(os.Args) > 1 {
		reader = file_reader(os.Args[1])
	} else {
		reader = aprs_reader()
	}

	each_message(reader, processor)
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
	auth := fmt.Sprintf("user %s pass -1 vers libfapGo 0.0.1 filter r/%s/%s/%s\n",
		os.Getenv("APRS_USER"),
		os.Getenv("AF_LAT"),
		os.Getenv("AF_LNG"),
		os.Getenv("APRS_RADIUS"),
	)
	fmt.Fprintf(c, auth)
}

func keepalive(c net.Conn) {
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		for t := range ticker.C {
			fmt.Fprintf(c, "# libfapGo keepalive %s\n", t)
		}
	}()
}

func each_message(r *bufio.Reader, processor func(packet *fap.Packet)) {
	for {
		line, err := r.ReadString('\n')
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
			} else {
			    log.Println(line)
				log.Fatal(err)
			}
		}
	}
}

func aprs_reader() *bufio.Reader {
	connection := connect()
	authenticate(connection)
	keepalive(connection)

	return bufio.NewReader(connection)
}

func file_reader(fn string) *bufio.Reader {
	fmt.Printf("Reading from %s\n", fn)

	f, err := os.Open(fn)
	if err != nil {
		panic(err)
	}

	return bufio.NewReader(f)
}
