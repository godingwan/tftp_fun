package tftpd

import (
	"io"
	"net"
)

type Server struct {
	readHandler  func(filename string, rf io.ReaderFrom) error
	writeHandler func(filename string, wt io.WriterTo) error
}

func NewServer(readHandler func(file string, rf io.ReaderFrom) error,
	writeHandler func(filename string, wt io.WriterTo) error) *Server {
	return &Server{
		readHandler:  readHandler,
		writeHandler: writeHandler,
	}
}

func (s *Server) ListenAndServe(addr string) error {
	addr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	return s.Serve(conn)
}

func (s *Server) Serve(conn *net.UDPConn) error {
	defer conn.Close()

	// size of byte differs
	buf := make([]byte, 4096)

	for {
		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			// TODO: retry logic maybe?
			return errors.Wrap(err, "failed to read from UDP connection")
		}
		packet, err := tftp.ParsePacket(buf)
		if err != nil {
			return errors.Wrap(err, "failed to parse the data")
		}
		switch packet.(type) {
		case tftp.PacketRequest:
		case tftp.PacketData:
		case tftp.PacketAck:
		case tftp.PacketError:
		}
	}
}
