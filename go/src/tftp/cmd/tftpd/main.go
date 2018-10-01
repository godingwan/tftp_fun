package main

import (
	"log"

	"github.com/jsungholee/tftp/tftp/go/src/tftp/server"
)

func main() {
	log.Println("Starting TFTP server")
	if err := server.NewServer().ListenAndServe(""); err != nil {
		log.Fatal("Server has failed")
	}
}
