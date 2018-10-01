package main

import (
	"log"
	"os"

	"github.com/jsungholee/tftp/tftp/go/src/tftp/server"
)

func main() {
	var addr string
	if len(os.Args) < 2 {
		addr = "localhost:69"
	} else {
		addr = os.Args[1]
	}
	log.Printf("Starting TFTP server on %s", addr)
	if err := server.NewServer().ListenAndServe(addr); err != nil {
		log.Fatal("Server has failed")
	}
}
