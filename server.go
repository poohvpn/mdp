package mdp

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/poohvpn/icmdp"
	"github.com/poohvpn/pooh"
	"github.com/rs/zerolog/log"
)

type ServerConfig struct {
	Port       int
	Obfuscator Obfuscator
}

func (c *ServerConfig) fix() {
	if c.Obfuscator == nil {
		c.Obfuscator = nopObfuscator{}
	}
}

func Listen(config ServerConfig) (s *Server, err error) {
	config.fix()
	s = &Server{
		config: config,
		sess: newSession().setLocalAddr(&Addr{
			Port: config.Port,
		}),
	}
	s.tcpListener, err = net.ListenTCP("tcp", &net.TCPAddr{
		Port: config.Port,
	})
	if err != nil {
		return
	}
	s.udpListener, err = net.ListenUDP("udp", &net.UDPAddr{
		Port: config.Port,
	})
	if err != nil {
		return
	}
	s.icmpV4Conn, err = icmdp.ListenICMDP("icmdp4", nil)
	if err != nil {
		log.Warn().Err(err).Msg("listen icmdp4")
		err = nil
	}
	s.icmpV6Conn, err = icmdp.ListenICMDP("icmdp6", nil)
	if err != nil {
		log.Warn().Err(err).Msg("listen icmdp6")
		err = nil
	}

	go icmdp.DisableLinuxEcho()
	go s.acceptTcpConn(s.tcpListener)
	go s.handlePacketConn(s.udpListener)
	go s.handlePacketConn(s.icmpV4Conn)
	go s.handlePacketConn(s.icmpV6Conn)
	return
}

var _ net.PacketConn = &Server{}

type Server struct {
	config      ServerConfig
	sess        *session
	tcpListener *net.TCPListener
	udpListener *net.UDPConn
	icmpV4Conn  *icmdp.Conn
	icmpV6Conn  *icmdp.Conn

	nat       sync.Map // uint32 -> *session
	closeOnce pooh.Once
	err       error
}

func (s *Server) acceptTcpConn(listener *net.TCPListener) {
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			s.err = err
			return
		}
		go s.handleTcpConn(s.config.Obfuscator.ObfuscateStreamConn(conn))
	}
}

func (s *Server) handleTcpConn(tcpConn net.Conn) {
	var err error
	defer func() {
		if err != nil {
			_ = tcpConn.Close()
		}
	}()
	conn := &tcpDatagram{
		Conn: pooh.NewConn(tcpConn, true),
	}
	id, err := conn.Uint32()
	if err != nil {
		return
	}

	s.loadOrStoreSession(id).upsert(conn)
	// todo may be remove ep from s.nat here
}

func (s *Server) handlePacketConn(conn net.PacketConn) {
	if pooh.IsNil(conn) {
		return
	}
	conn = s.config.Obfuscator.ObfuscatePacketConn(conn)
	buf := make([]byte, pooh.BufferSize)
	for {
		if s.closeOnce.Done() {
			return
		}
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			s.err = err
			return
		}
		if n < idSize {
			continue
		}
		go s.handlePacket(pooh.Duplicate(buf[:n]), addr, conn)
	}
}

func (s *Server) handlePacket(p []byte, raddr net.Addr, conn net.PacketConn) {
	data, id := p[:len(p)-idSize], binary.BigEndian.Uint32(p[len(p)-idSize:])
	sess := s.loadOrStoreSession(id)
	ep := sess.upsert(&serverWriteConn{
		remote:     raddr,
		packetConn: conn,
	})
	ep.recv(data)
}

func (s *Server) loadOrStoreSession(id uint32) *session {
	v, _ := s.nat.LoadOrStore(id, &session{
		id:         id,
		packetCh:   s.sess.packetCh,
		serverSide: true,
	})
	return v.(*session)
}

// todo clean up s.nat

func (s *Server) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	if s.err != nil {
		return 0, nil, s.err
	}
	packet := <-s.sess.packetCh
	return copy(p, packet.Data), packet.Addr, nil
}

func (s *Server) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if s.err != nil {
		return 0, s.err
	}
	n = len(p)
	a := addr.(*Addr)
	err = a.sess.send(p)
	return
}

func (s *Server) close() error {
	return pooh.Close(s.tcpListener, s.udpListener, s.icmpV4Conn, s.icmpV6Conn)
}

func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.err = s.close()
	})
	return s.err
}

func (s *Server) LocalAddr() net.Addr {
	return s.sess.localAddr
}

func (s *Server) SetDeadline(t time.Time) error {
	panic("implement me")
}

func (s *Server) SetReadDeadline(t time.Time) error {
	panic("implement me")
}

func (s *Server) SetWriteDeadline(t time.Time) error {
	panic("implement me")
}
