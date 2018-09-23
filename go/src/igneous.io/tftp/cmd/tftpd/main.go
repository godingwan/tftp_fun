package main

import (
	"log"
	"net"
)

func main() {
	// Connect udp
	conn, err := net.Dial("udp", "host:port")
	if err != nil {
		log.Fatal("Failed to create udp connection")
	}
	defer conn.Close()

	// TODO implement the in-memory tftp server
}
