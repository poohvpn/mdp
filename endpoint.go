package mdp

import (
	"io"
	"net"
	"time"

	"github.com/poohvpn/pooh"
	"github.com/rs/zerolog/log"
)

type endpoint struct {
	super     *session
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
			Msg("endpoint.send")
	}
	defer func() {
		if err != nil {
			e.sendCount++
		}
	}()
	_, err = e.conn.Write(data)
	return
}

func (e *endpoint) recv(data []byte) (ok bool) {
	e.lastRecv = time.Now()
	e.recvCount++
	return e.super.recv(data)
}

func (e *endpoint) run() {
	switch conn := e.conn.(type) {
	case *serverWriteConn:
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
