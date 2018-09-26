package server

import (
	"io"
	"net"

	"github.com/jsungholee/tftp/tftp/go/src/igneous.io/tftp"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
)

const (
	errCodeNotDefined uint16 = iota
	errAlreadyExist
)

const (
	buffSize = 4096
)

type Server struct {
	readHandler  func(filename string, rf io.ReaderFrom) error
	writeHandler func(filename string, wt io.WriterTo) error
	cache        cache.Cache
}

func NewServer(readHandler func(file string, rf io.ReaderFrom) error,
	writeHandler func(filename string, wt io.WriterTo) error) *Server {
	return &Server{
		readHandler:  readHandler,
		writeHandler: writeHandler,
		cache: cache.New(cache.NoExpiration, 0)
	}
}

func (s *Server) ListenAndServe(addr string) error {
	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", a)
	if err != nil {
		return err
	}
	return s.Serve(conn)
}

func (s *Server) Serve(conn *net.UDPConn) error {
	defer conn.Close()

	// size of byte differs
	buf := make([]byte, buffSize)

	for {
		_, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			// TODO: retry logic maybe?
			return errors.Wrap(err, "failed to read from UDP connection")
		}
		packet, err := tftp.ParsePacket(buf)
		if err != nil {
			return errors.Wrap(err, "failed to parse the data")
		}
		switch pkt := packet.(type) {
		case *tftp.PacketRequest:
			conn, err := net.DialUDP("udp", nil, raddr)
			if err != nil {
				errPkt := createErrorPacket(errCodeNotDefined, "Something went wrong")
				data := errPkt.Serialize()
				if _, err := conn.Write(data); err != nil {
					log.Println(errors.Wrap(err, "Error writing to UDP"))
				}
				return err
			}
			if pkt.Op == tftp.OpWRQ {
				s.handleWriteReq(conn, pkt)
			}
		case *tftp.PacketData:
		case *tftp.PacketAck:
		case *tftp.PacketError:
		}
	}
}

func (s *Server) handleWriteReq(conn *net.UDPConn, req *tftp.PacketRequest) {
	// check if file already exist in memory
	file, found := s.cache.Get(req.Filename)
	if found {
		errPkt := createErrorPacket(errAlreadyExist, "This file already exists")
		data := errPkt.Serialize()
		conn.Write(data)
		return
	}

	// send an ack packet
	ack := &tftp.PacketAck{}
	data := ack.Serialize()
	conn.Write(data)
	ack.BlockNum++

	buf := make([]byte, buffSize)
	for {
		
	}
}

func createErrorPacket(code uint16, msg string) *tftp.PacketError {
	return &tftp.PacketError{
		Code: code,
		Msg:  msg,
	}
}
