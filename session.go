package mdp

import (
	"errors"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/poohvpn/icmdp"
	"github.com/poohvpn/pooh"
	"github.com/rs/zerolog/log"
)

func newSession() *session {
	return &session{
		id:       rand.Uint32(),
		packetCh: make(chan rawPacket, queueSize),
	}
}

type session struct {
	id         uint32
	localAddr  *Addr
	remoteAddr *Addr
	lastSeen   time.Time
	endpoints  sync.Map
	packetCh   chan rawPacket
	closeOnce  pooh.Once
	closeErr   error
	serverSide bool
}

// todo remove tcp conn itself from session

func (s *session) upsert(conn net.Conn) *endpoint {
	if s.serverSide && s.remoteAddr == nil {
		s.remoteAddr = fromNetAddr(conn.RemoteAddr())
		s.remoteAddr.sess = s
	}
	s.lastSeen = time.Now()
	v, exist := s.endpoints.LoadOrStore(endpointIndex(conn), &endpoint{
		super: s,
	})
	ep := v.(*endpoint)
	if !exist {
		ep.conn = conn
		go ep.run()
	}
	return ep
}

func (s *session) mostRecentEndpoint() (res *endpoint) {
	var t time.Time
	s.endpoints.Range(func(_, v interface{}) bool {
		ep := v.(*endpoint)
		if ep.lastRecv.After(t) || t == (time.Time{}) {
			t = ep.lastRecv
			res = ep
		}
		return true
	})
	return
}

func (s *session) dial(network string, remote *Addr) (err error) {
	var conn net.Conn
	switch network {
	case "tcp4", "tcp6":
		conn, err = dialTcpDatagram(&net.TCPAddr{
			IP:   remote.IP,
			Port: remote.Port,
			Zone: remote.Zone,
		}, s.id)
	case "udp4", "udp6":
		conn, err = net.DialUDP("udp", nil, &net.UDPAddr{
			IP:   remote.IP,
			Port: remote.Port,
			Zone: remote.Zone,
		})
	case "icmdp4":
		conn, err = icmdp.DialICMDP("udp4", nil, &icmdp.Addr{
			IP: remote.IP,
		})
		if err != nil {
			conn, err = icmdp.DialICMDP("icmdp4", nil, &icmdp.Addr{
				IP: remote.IP,
			})
		}
	case "icmdp6":
		conn, err = icmdp.DialICMDP("udp6", nil, &icmdp.Addr{
			IP:   remote.IP,
			Zone: remote.Zone,
		})
		if err != nil {
			conn, err = icmdp.DialICMDP("icmdp6", nil, &icmdp.Addr{
				IP:   remote.IP,
				Zone: remote.Zone,
			})
		}
	default:
		panic("unknown network " + network)
	}
	if err != nil {
		return
	}
	switch network {
	case "udp4", "udp6", "icmdp4", "icmdp6":
		conn = &clientPacketConn{
			Conn: conn,
			id:   s.id,
		}
	}
	if s.localAddr == nil {
		s.setLocalAddr(conn.LocalAddr())
	}
	if s.remoteAddr == nil {
		s.setRemoteAddr(remote)
	}

	// append new conn to endpoints
	ep := &endpoint{
		super: s,
		conn:  conn,
	}
	s.endpoints.Store(endpointIndex(conn), ep)
	go ep.run()
	return
}

func (s *session) send(data []byte) error {
	if debug {
		log.Debug().Uint32("id", s.id).Bytes("data", data).Msg("session.send")
	}
	ep := s.mostRecentEndpoint()
	if ep == nil {
		if debug {
			log.Warn().Str("addr", s.remoteAddr.String()).Msg("no most recent endpoint")
		}
		return errors.New("noa available endpoint to send")
	}
	return ep.send(data)
}

func (s *session) recv(data []byte) (ok bool) {
	if debug {
		log.Debug().Uint32("id", s.id).Bytes("data", data).Msg("session.recv")
	}
	if len(data) == 0 {
		return true
	}
	select {
	case <-s.closeOnce.Wait():
		return false
	case s.packetCh <- rawPacket{Addr: s.remoteAddr, Data: data}:
	}
	return true
}

func (s *session) close() {
	if debug {
		log.Debug().Uint32("id", s.id).Msg("session.close")
	}
	errs := new(multierror.Error)
	s.endpoints.Range(func(_, v interface{}) bool {
		ep := v.(*endpoint)
		errs = multierror.Append(errs, ep.conn.Close())
		return true
	})
	s.closeErr = errs.ErrorOrNil()
	return
}

func (s *session) Close() error {
	s.closeOnce.Do(s.close)
	return s.closeErr
}

func (s *session) setLocalAddr(netAddr net.Addr) *session {
	addr := fromNetAddr(netAddr)
	addr.sess = s
	s.localAddr = addr
	return s
}

func (s *session) setRemoteAddr(netAddr net.Addr) *session {
	addr := fromNetAddr(netAddr)
	addr.sess = s
	s.remoteAddr = addr
	return s
}
