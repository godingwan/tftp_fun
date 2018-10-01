In-memory TFTP Server
=====================

This is a simple in-memory TFTP server, implemented in Go.  It is
RFC1350-compliant, but doesn't implement the additions in later RFCs.  In
particular, options are not recognized.

Usage
-----
Grab the project from https://github.com/jsungholee/tftp_fun (go get github.com/jsungholee/tftp_fun). 

To run the server...
- Go to root of directory
- type `go run go/src/tftp/cmd/tftpd/main.go`
- Note: you can also add an optional address as a parameter or it will default to localhost:69
- Request logs will be outputted to LogFile.txt in root dir


Testing
-------
TODO
