package mdp

import (
	"io"
	"net"
	"time"

	"github.com/poohvpn/icmdp"
	"github.com/poohvpn/pooh"
	"github.com/rs/zerolog/log"
)

type endpointType byte

const (
	endpointTCP   endpointType = 0x6
	endpointUDP   endpointType = 0x11
	endpointICMDP endpointType = 0x1
)

// auto-reconnect
type endpoint struct {
	index     uint64
	typ       endpointType
	dst       bool
	addr      *Addr
	conn      net.Conn
	lastRecv  time.Time
	lastSent  time.Time
	recvCount int
	sendCount int
}

func (e *endpoint) send(data []byte) (err error) {
	if debug {
		log.Debug().
			Str("local", e.conn.LocalAddr().String()).
			Str("remote", e.conn.RemoteAddr().String()).
			Bytes("data", data).
			Msg("endpoint.output")
	}
	defer func() {
		if err != nil {
			e.sendCount++
		}
	}()
	_, err = e.conn.Write(data)
	return
}

func (e *endpoint) recv(data []byte) bool {
	e.lastRecv = time.Now()
	e.recvCount++
	return e.addr.sess.input(data, e.dst)
}

func (e *endpoint) run() {
	if e.dst {
		e.forwardLoop()
	} else {
		e.inputLoop()
	}
	e.drop()
}

func (e *endpoint) inputLoop() {
	switch conn := e.conn.(type) {
	case *writeOnlyConn:
		// does not need to handle server side PacketConn, which is already received by PacketConn loop
		return
	default:
		defer e.conn.Close()
		buf := make([]byte, pooh.BufferSize)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if debug && err != io.EOF {
					log.Debug().Err(err).Msg("mdp: endpoint.conn is broken")
				}
				return
			}
			if !e.recv(pooh.Duplicate(buf[:n])) {
				return
			}
		}
	}
}

func (e *endpoint) forwardLoop() {
	for {

	}
}

func (e *endpoint) dial() {
	var (
		conn net.Conn
		err  error
	)
	switch e.typ {
	case endpointTCP:
		conn, err = dialTcpDatagram(
			&net.TCPAddr{
				IP:   e.addr.IP,
				Port: e.addr.Port,
				Zone: e.addr.Zone,
			},
			e.addr.sess.config.SessionID,
			e.addr.sess.config.Obfuscator,
		)
	case endpointUDP:
		conn, err = net.DialUDP("udp", nil, &net.UDPAddr{
			IP:   e.addr.IP,
			Port: e.addr.Port,
			Zone: e.addr.Zone,
		})
	case endpointICMDP:
		if pooh.IsIPv4(e.addr.IP) {
			conn, err = icmdp.DialICMDP("udp4", nil, &icmdp.Addr{
				IP: e.addr.IP,
			})
			if err != nil {
				conn, err = icmdp.DialICMDP("icmdp4", nil, &icmdp.Addr{
					IP: e.addr.IP,
				})
			}
		} else {
			conn, err = icmdp.DialICMDP("udp6", nil, &icmdp.Addr{
				IP:   e.addr.IP,
				Zone: e.addr.Zone,
			})
			if err != nil {
				conn, err = icmdp.DialICMDP("icmdp6", nil, &icmdp.Addr{
					IP:   e.addr.IP,
					Zone: e.addr.Zone,
				})
			}
		}
	default:
		panic(e.typ)
	}
	if err != nil {
		return
	}
	switch network {
	case "udp4", "udp6", "icmdp4", "icmdp6":
		conn = ob.ObfuscateDatagramConn(conn)
	}
	if s.localAddr == nil {
		s.setInputAddr(conn.LocalAddr())
	}
	if s.srcAddr == nil {
		s.setInputAddr(remote)
	}

	// append new conn to srcEndpoints
	ep := &endpoint{
		super: s,
		conn:  conn,
	}
	s.srcEndpoints.Store(connIndex(conn), ep)
	go ep.run()
	return
}

func (e *endpoint) drop() {
	if e.dst {
		e.addr.sess.srcEndpoints.Delete(e.index)
	} else {
		e.addr.sess.dstEndpoints.Delete(e.index)
	}
}

func (e *endpoint) available() bool {
	panic("implement me")
}
