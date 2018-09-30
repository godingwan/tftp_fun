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
	blockSize = 512
)

type Server struct {
	readHandler  func(filename string, rf io.ReaderFrom) error
	writeHandler func(filename string, wt io.WriterTo) error
	cache        cache.Cache
}

type request struct {
	addr *net.UDPAddr
	conn *net.UDPConn
}

type file struct {
	name string
	data []byte
	readable bool
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
	defer conn.Close()

	req := &request{a, conn}
	return s.Serve(req)
}

func (s *Server) Serve(req *request) error {
	// size of byte differs
	buf := make([]byte, buffSize)

	for {
		_, raddr, err := req.conn.ReadFromUDP(buf)
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
				err := createAndSendErrorPacket(errCodeNotDefined, "Something went wrong")
				return errors.Wrap(err, "Failed to send error packet")
			}
			
			if pkt.Op == tftp.OpWRQ {
				go s.handleWriteReq(raddr, pkt)
			}
		case *tftp.PacketData:
		case *tftp.PacketAck:
		case *tftp.PacketError:
		}
	}
}

func (s *Server) handleWriteReq(caddr *net.UDPAddr, req *tftp.PacketRequest) {
	waddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		log.Println("Failed getting UDP address to write to: ", err)
		return
	}
	conn, err := net.DialUDP("udp", waddr, caddr)
	if err != nil {
		log.Println("Error dialing UDP to client: ", err)
		return
	}

	// check if file already exist in memory
	file, found := s.cache.Get(req.Filename)
	if found {
		createAndSendErrorPacket(conn, errAlreadyExist, "This file already exists")
		return
	}

	// send an ack packet
	ack := &tftp.PacketAck{}
	ack.sendAck(conn)

	buf := make([]byte, buffSize)
	for {
		_, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Failed to read data from udp connection")
			createAndSendErrorPacket(conn, errCodeNotDefined, "Something went wrong")
			return
		}
		packet, err := tftp.ParsePacket(buf)
		if err != nil {
			log.Println("Failed to parse the data")
			return
		}

		switch pkt := packet.(type) {
		case *tftp.PacketData:
			if pkt.BlockNum == 1 { // first packet so must initialize
				file := &file{name: req.Filename, data: pkt.Data}
				s.cache.Set(req.Filename, file, cache.NoExpiration)
				ack.sendAck(conn)
			} else { // file exist in map
				var fileData *file
				if pkt.BlockNum == ack.BlockNum { // check BlockNum matches
					file, ok := s.cache.Get(req.Filename)
					if !ok {
						log.Printf("Could not find data for file %s", req.Filename)
						return
					}
					fileData, ok := file.(*file)
					if !ok {
						log.Println("Failed to type assert data into []byte")
						return
					}
					fileData.data = append(fileData.data, pkt.Data...)
					ack.sendAck(conn)
				}
				// Check if it's it's the last data to be transferred
				if len(pkt.Data) < 512 { // as defined in the RFC
					log.Printf("File [%s] is ready to be read", fileData.name)
					fileData.readable = true
					s.cache.Set(req.Filename, fileData, cache.NoExpiration)
					return
				}
				s.cache.Set(req.Filename, fileData, cache.NoExpiration)
			}
		default:
			log.Println("Received a non data packet for a write request")
			return
		}
	}
}

func createAndSendErrorPacket(conn *net.UDPConn, code uint16, msg string) error {
	pkt := &tftp.PacketError{
		Code: code,
		Msg:  msg,
	}
	data := pkt.Serialize()
	if _, err := conn.Write(data); err != nil {
		log.Println(errors.Wrap(err, "Error writing to UDP"))
		return err
	}
	return nil
}

func (p *tftp.PacketAck) sendAck(conn *net.UDPConn) {
	data := p.Serialize()
	conn.Write(data)
	p.BlockNum++
}