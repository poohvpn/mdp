package mdp

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/poohvpn/icmdp"
	"github.com/poohvpn/pooh"
	"github.com/rs/zerolog/log"
)

func Listen(port int) (s *Server, err error) {
	s = &Server{
		session: newSession(Config{}).
			setSrcInputCh(nil).
			setInputAddr(&Addr{
				Port: port,
			}),
	}
	s.tcpListener, err = net.ListenTCP("tcp", &net.TCPAddr{
		Port: port,
	})
	if err != nil {
		return
	}
	s.udpListener, err = net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
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

	if s.icmpV4Conn != nil || s.icmpV6Conn != nil {
		go icmdp.DisableLinuxEcho()
	}
	go s.acceptTcpConn(s.tcpListener)
	go s.handlePacketConn(s.udpListener)
	go s.handlePacketConn(s.icmpV4Conn)
	go s.handlePacketConn(s.icmpV6Conn)
	return
}

var _ net.PacketConn = &Server{}

type Server struct {
	session         *session
	tcpListener     *net.TCPListener
	udpListener     *net.UDPConn
	icmpV4Conn      *icmdp.Conn
	icmpV6Conn      *icmdp.Conn
	forwardNodes    sync.Map // uint32 -> DualStackAddr
	inputSessions   sync.Map // uint32 -> *session
	forwardSessions sync.Map // uint64 -> *session
	closeOnce       pooh.ErrorOnce
}

func (s *Server) acceptTcpConn(listener *net.TCPListener) {
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			return
		}
		go s.handleTcpConn(s.session.config.Obfuscator.ObfuscateStreamConn(conn))
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
	sid, nid, err := readStreamIDs(conn)
	if err != nil {
		return
	}
	sess, ok := s.upsertSession(sid, nid)
	if !ok {
		return
	}
	sess.upsertInputConn(endpointTCP, conn)
}

func (s *Server) handlePacketConn(conn net.PacketConn) {
	var typ endpointType
	switch conn.(type) {
	case *net.UDPConn:
		typ = endpointUDP
	case *icmdp.Conn:
		typ = endpointICMDP
	}
	if pooh.IsNil(conn) {
		return
	}
	conn = s.session.config.Obfuscator.ObfuscatePacketConn(conn)
	buf := make([]byte, pooh.BufferSize)
	for {
		if s.closeOnce.Done() {
			return
		}
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < sessionIDSize+nodeIDSize {
			continue
		}
		go s.handlePacket(pooh.Duplicate(buf[:n]), addr, typ, conn)
	}
}

func (s *Server) handlePacket(p []byte, raddr net.Addr, typ endpointType, conn net.PacketConn) {
	woc := &writeOnlyConn{
		remote:     raddr,
		packetConn: conn,
	}
	sid, nid, data := readPacketIDs(p)
	sess, ok := s.upsertSession(sid, nid)
	if !ok {
		return
	}
	ep := sess.upsertInputConn(typ, woc)
	ep.recv(data)
}

func (s *Server) upsertSession(sid, nid uint32) (*session, bool) {
	if nid == s.session.config.NodeID { // input
		v, ok := s.inputSessions.Load(sid) // fast load
		if !ok {
			v, _ = s.inputSessions.LoadOrStore(sid,
				newSession(Config{
					SessionID:  sid,
					NodeID:     s.session.config.NodeID,
					Obfuscator: s.session.config.Obfuscator,
				}).setSrcInputCh(s.session.srcInputCh))
		}
		return v.(*session), true
	}

	// frontward forward
	index := forwardIndex(sid, nid)
	v, ok := s.forwardSessions.Load(index) // fast load
	if !ok {
		v, ok = s.forwardNodes.Load(nid)
		if !ok {
			return nil, false
		}
		addr := v.(DualStackAddr)
		v, ok = s.forwardSessions.LoadOrStore(index,
			newSession(Config{
				SessionID:     sid,
				NodeID:        nid,
				DualStackAddr: addr,
				Obfuscator:    s.session.config.Obfuscator,
			}).setSrcInputCh(nil))
		if !ok {
			v.(*session).addForwardEndpoints()
		}
	}
	return v.(*session), true
}

// todo clean up s.inputSessions

func (s *Server) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	if s.closeOnce.Done() {
		return 0, nil, io.EOF
	}
	packet := <-s.session.srcInputCh
	return copy(p, packet.Data), packet.Addr, nil
}

func (s *Server) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if s.closeOnce.Done() {
		return 0, io.EOF
	}
	n = len(p)
	a := addr.(*Addr)
	err = a.sess.output(p, false)
	return
}

func (s *Server) close() error {
	return pooh.Close(s.tcpListener, s.udpListener, s.icmpV4Conn, s.icmpV6Conn)
}

func (s *Server) Close() error {
	return s.closeOnce.Do(func() error {
		return pooh.Close(s.tcpListener, s.udpListener, s.icmpV4Conn, s.icmpV6Conn)
	})
}

func (s *Server) LocalAddr() net.Addr {
	return s.session.srcAddr
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

func (s *Server) SetNodeID(id uint32) *Server {
	s.session.config.NodeID = id
	return s
}

func (s *Server) SetObfuscator(ob Obfuscator) *Server {
	s.session.config.Obfuscator = ob
	return s
}

func (s *Server) SetForwardNode(id uint32, addr DualStackAddr) *Server {
	if addr.invalid() {
		s.forwardNodes.Delete(id)
	} else {
		s.forwardNodes.Store(id, addr)
	}
	return s
}

func (s *Server) DeleteForwardNode(id uint32) *Server {
	s.forwardNodes.Delete(id)
	return s
}
