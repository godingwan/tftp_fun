package server

import (
	"log"
	"net"
	"os"

	"github.com/jsungholee/tftp/tftp/go/src/tftp"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	errCodeNotDefined uint16 = iota
	errAlreadyExist
	errDoesNotExist
	errFileNotReady
	errRecErr
)

const (
	buffSize  = 1024
	blockSize = 512
)

// Server struct contains data for serving tftp functionality
type Server struct {
	cache      *cache.Cache
	fileLogger *logrus.Logger
}

type request struct {
	addr *net.UDPAddr
	conn *net.UDPConn
}

type file struct {
	name     string
	data     []byte
	readable bool
}

// NewServer will create an instance of a server to do TFTP things
func NewServer() *Server {
	// setting up logger
	logger := logrus.New()
	f, err := os.OpenFile("LogFile.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file for logging: %v", err)
	}
	defer f.Close()
	logger.SetOutput(f)
	return &Server{
		cache:      cache.New(cache.NoExpiration, 0),
		fileLogger: logger,
	}
}

// ListenAndServe will start a udp conn and call serve
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

// Serve function will handle the routing of incoming packets
func (s *Server) Serve(req *request) error {
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
			if pkt.Op == tftp.OpWRQ {
				s.fileLogger.Info("User submitted a write req")
				go s.handleWriteReq(raddr, pkt)
			} else if pkt.Op == tftp.OpRRQ {
				s.fileLogger.Info("User submitted a read request")
				go s.handleReadReq(raddr, pkt)
			}
		default:
			s.fileLogger.Error("User submitted an invalid request")
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
	defer conn.Close()

	// check if file already exist in memory
	_, found := s.cache.Get(req.Filename)
	if found {
		CreateAndSendErrorPacket(conn, errAlreadyExist, "This file already exists")
		return
	}

	// send an ack packet
	ack := &tftp.PacketAck{}
	SendAck(conn, ack)

	buf := make([]byte, buffSize)
	for {
		_, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Failed to read data from udp connection")
			CreateAndSendErrorPacket(conn, errCodeNotDefined, "Something went wrong")
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
				f := &file{name: req.Filename, data: pkt.Data}
				s.cache.Set(req.Filename, f, cache.NoExpiration)
				SendAck(conn, ack)
			} else { // file exist in map
				var fileData *file
				if pkt.BlockNum == ack.BlockNum { // check BlockNum matches
					f, found := s.cache.Get(req.Filename)
					if !found {
						log.Printf("Could not find data for file %s", req.Filename)
						return
					}
					fileData, ok := f.(*file)
					if !ok {
						log.Println("Failed to type assert data into []byte")
						return
					}
					fileData.data = append(fileData.data, pkt.Data...)
					SendAck(conn, ack)
				}
				// Check if it's it's the last data to be transferred
				if len(pkt.Data) < blockSize { // as defined in the RFC
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

func (s *Server) handleReadReq(caddr *net.UDPAddr, req *tftp.PacketRequest) {
	rdAddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		log.Println("Failed getting UDP address to write to: ", err)
		return
	}
	conn, err := net.DialUDP("udp", rdAddr, caddr)
	if err != nil {
		log.Println("Error dialing UDP to client: ", err)
		return
	}
	defer conn.Close()

	// check if file already exist in memory
	f, found := s.cache.Get(req.Filename)
	if !found {
		log.Printf("File [%s] does not exist", req.Filename)
		CreateAndSendErrorPacket(conn, errDoesNotExist, "The file does not exist")
		return
	}

	fileData, ok := f.(*file)
	if !ok {
		log.Println("Failed to type assert data into []byte")
		return
	}

	// check if file is not readable
	if !fileData.readable {
		log.Println("File is not ready to be read yet")
		CreateAndSendErrorPacket(conn, errFileNotReady, "File has not completed transfer full transfer yet")
		return
	}

	buf := make([]byte, buffSize)
	var start, end int
	blockNum := uint16(1)
	for start = 0; len(fileData.data) > blockSize; start += blockSize {
		end = start + blockSize
		dataPkt := &tftp.PacketData{BlockNum: blockNum}
		if end > len(fileData.data) { // leftover data not 512 length
			dataPkt.Data = fileData.data[start:]
			if err := SendDataPacket(conn, dataPkt); err != nil {
				log.Println("Failed to send data")
				return
			}
		} else {
			dataPkt.Data = fileData.data[start:end]
			if err := SendDataPacket(conn, dataPkt); err != nil {
				log.Println("Failed to send data")
				return
			}
		}

		// receive resp from remote
		_, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Error reading data from remote", err)
			CreateAndSendErrorPacket(conn, errRecErr, "Error in receiving message")
			return
		}
		packet, err := tftp.ParsePacket(buf)
		if err != nil {
			log.Println("Failed to parse the data")
			return
		}

		switch pkt := packet.(type) {
		case *tftp.PacketAck:
			if pkt.BlockNum == blockNum {
				blockNum++
			} else {
				log.Println("Incorrect block num received")
				return
			}
		default:
			log.Println("Don't know how to handle this response")
			return
		}
		log.Println("File transfer is complete")
	}
}

// CreateAndSendErrorPacket will send an error packet to the provided connection
func CreateAndSendErrorPacket(conn *net.UDPConn, code uint16, msg string) error {
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

// SendDataPacket will send a data packet to the provided connection
func SendDataPacket(conn *net.UDPConn, pkt *tftp.PacketData) error {
	data := pkt.Serialize()
	if _, err := conn.Write(data); err != nil {
		return err
	}
	return nil
}

// SendAck will an acknowledgement packet to the provided connection
func SendAck(conn *net.UDPConn, pkt *tftp.PacketAck) {
	data := pkt.Serialize()
	conn.Write(data)
	pkt.BlockNum++
}
