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

type Config struct {
	SessionID      uint32
	NodeID         uint32
	ForwardNodeIDs []uint32
	DualStackAddr  DualStackAddr
	Threads        int
	DisableICMDP   bool
	DisableTCP     bool
	DisableUDP     bool
	Obfuscator     Obfuscator
}

func (c *Config) def() Config {
	for c.SessionID == 0 {
		c.SessionID = rand.Uint32()
	}
	if c.Threads <= 0 {
		c.Threads = 1
	} else if c.Threads > 32 {
		c.Threads = 32
	}
	if c.Obfuscator == nil {
		c.Obfuscator = nopObfuscator{}
	}
	if c.DisableTCP && c.DisableUDP && c.DisableICMDP {
		c.DisableUDP = false
	}
	return *c
}

func newSession(config Config) *session {
	s := &session{
		config: config.def(),
	}
	return s
}

type session struct {
	config       Config
	srcAddr      *Addr
	srcEndpoints sync.Map // uint64 -> *endpoint
	srcInputCh   chan *inputPacket
	dstAddr      *Addr
	dstEndpoints sync.Map // uint64 -> *endpoint
	dstInputCh   chan *inputPacket
	activeAt     time.Time
	closeOnce    pooh.ErrorOnce
}

// todo remove tcp conn itself from session

func (s *session) setSrcInputCh(ch chan *inputPacket) *session {
	if ch == nil {
		ch = make(chan *inputPacket, queueSize)
	}
	s.srcInputCh = ch
	return s
}

func (s *session) addForwardEndpoints() *session {
	if s.dstInputCh == nil {
		s.dstInputCh = make(chan *inputPacket, queueSize)
	}
	config := s.config
	add := func(typ endpointType, addr DualStackAddr, threadIndex uint16) {
		if pooh.IsIPv4(addr.IP4) {
			s.addForwardEndpoint(typ, &Addr{
				IP:   addr.IP4,
				Port: addr.Port,
				sess: s,
			}, threadIndex)
		}
		if pooh.IsIPv6(addr.IP6) {
			s.addForwardEndpoint(typ, &Addr{
				IP:   addr.IP6,
				Port: addr.Port,
				Zone: addr.Zone,
				sess: s,
			}, threadIndex)
		}
	}
	for i := 0; i < config.Threads; i++ {
		if !config.DisableUDP {
			add(endpointUDP, config.DualStackAddr, uint16(i))
		}
		if !config.DisableTCP {
			add(endpointTCP, config.DualStackAddr, uint16(i))
		}
		if !config.DisableICMDP {
			add(endpointICMDP, config.DualStackAddr, uint16(i))
		}
	}
	return s
}

func (s *session) upsertInputConn(typ endpointType, conn net.Conn) *endpoint {
	if s.srcAddr == nil {
		s.setInputAddr(conn.RemoteAddr())
	}
	index := connIndex(conn)
	v, ok := s.srcEndpoints.Load(index) // fast load
	if !ok {
		v, ok = s.srcEndpoints.LoadOrStore(index, &endpoint{
			index: index,
			typ:   typ,
			addr:  s.srcAddr,
		})
		ep := v.(*endpoint)
		if !ok {
			ep.conn = conn
			go ep.inputLoop()
		}
	}
	s.activeAt = time.Now()
	return v.(*endpoint)
}

func (s *session) mostRecentEndpoint(dst bool) (res *endpoint) {
	eps := &s.srcEndpoints
	if dst {
		eps = &s.dstEndpoints
	}
	var t time.Time
	eps.Range(func(_, v interface{}) bool {
		ep := v.(*endpoint)
		if ep.lastRecv.After(t) || t == (time.Time{}) {
			t = ep.lastRecv
			res = ep
		}
		return true
	})
	return
}

func (s *session) addForwardEndpoint(typ endpointType, remote *Addr, threadIndex uint16) {
	index := endpointIndex(pooh.IsIPv4(remote.IP), typ, threadIndex, uint16(remote.Port))
	ep := &endpoint{
		index: index,
		typ:   typ,
		dst:   true,
		addr:  remote,
	}
	s.dstEndpoints.Store(index, ep)
	go ep.run()
}


func (s *session) dial(network string, remote *Addr, ob Obfuscator) (err error) {
	var conn net.Conn
	switch network {
	case "tcp4", "tcp6":
		conn, err = dialTcpDatagram(&net.TCPAddr{
			IP:   remote.IP,
			Port: remote.Port,
			Zone: remote.Zone,
		}, s.sid, ob)
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

func (s *session) input(data []byte, dst bool) bool {
	ch, addr := s.srcInputCh, s.srcAddr
	if dst {
		ch, addr = s.dstInputCh, s.dstAddr
	}
	if debug {
		log.Debug().
			Uint32("sid", s.config.SessionID).
			Bytes("data", data).
			Msg("session.input")
	}
	if len(data) == 0 {
		return true
	}
	select {
	case <-s.closeOnce.Wait():
		return false
	case ch <- &inputPacket{Addr: addr, Data: data}:
		return true
	}
}

func (s *session) output(data []byte, dst bool) error {
	if debug {
		log.Debug().
			Uint32("sid", s.config.SessionID).
			Uint32("nid", s.config.NodeID).
			Bytes("data", data).
			Msg("session.output")
	}
	ep := s.mostRecentEndpoint(dst)
	if ep == nil {
		if debug {
			log.Warn().Str("addr", s.srcAddr.String()).Msg("no most recent endpoint")
		}
		return errors.New("no available endpoint to output")
	}
	packet := make([]byte, 0, len(data)+sessionIDSize+nodeIDSize)
	packet = append(packet, data...)
	packet = append(packet, pooh.Uint322Bytes(s.config.NodeID)...)

	return ep.send(data)
}

func (s *session) forward(dst bool) {
	ch := s.srcInputCh
	if dst {
		ch = s.dstInputCh
	}
	for {
		select {
		case <-s.closeOnce.Wait():
			return
		case p := <-ch:
			// write to forward endpoints
			err := s.output(p.Data, !dst)
			if debug && err != nil {
				log.Debug().Err(err).Msg("forward output")
			}
		}
	}
}

func (s *session) Close() error {
	return s.closeOnce.Do(func() error {
		if debug {
			log.Debug().Uint32("sid", s.config.SessionID).Msg("session.close")
		}
		errs := new(multierror.Error)
		s.srcEndpoints.Range(func(_, v interface{}) bool {
			ep := v.(*endpoint)
			errs = multierror.Append(errs, ep.conn.Close())
			return true
		})
		return errs.ErrorOrNil()
	})
}

func (s *session) setInputAddr(netAddr net.Addr) *session {
	addr := fromNetAddr(netAddr)
	addr.sess = s
	s.srcAddr = addr
	return s
}

func (s *session) setForwardAddr(netAddr net.Addr) *session {
	addr := fromNetAddr(netAddr)
	addr.sess = s
	s.dstAddr = addr
	return s
}
