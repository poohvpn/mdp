package mdp

import (
	"net"
	"sync"
	"time"

	"github.com/poohvpn/pooh"
)

func dialTcpDatagram(addr *net.TCPAddr, id uint32, ob Obfuscator) (conn *tcpDatagram, err error) {
	tcpConn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return
	}
	conn = &tcpDatagram{
		Conn: pooh.NewConn(ob.ObfuscateStreamConn(tcpConn), true),
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	err = conn.Conn.WriteUint32(id)
	if err != nil {
		return
	}
	return
}

var _ net.Conn = &tcpDatagram{}

type tcpDatagram struct {
	pooh.Conn
	writeM sync.Mutex
}

// todo reconnect

func (td *tcpDatagram) Read(b []byte) (n int, err error) {
	length, err := td.Uint16()
	if err != nil {
		return
	}
	data, err := td.Bytes(int(length))
	if err != nil {
		return
	}
	n = copy(b, data)
	return
}

func (td *tcpDatagram) Write(b []byte) (n int, err error) {
	td.writeM.Lock()
	defer td.writeM.Unlock()
	_, err = td.Conn.Write(append(pooh.Int2Bytes(len(b), 2), b...))
	n = len(b)
	return
}

func (td *tcpDatagram) Close() error {
	return td.Conn.Close()
}

var _ net.Conn = &writeOnlyConn{}

type writeOnlyConn struct {
	remote     net.Addr
	packetConn net.PacketConn
}

func (p *writeOnlyConn) Read(b []byte) (n int, err error) {
	panic("shouldn't read from writeOnlyConn")
}

func (p *writeOnlyConn) Write(b []byte) (n int, err error) {
	return p.packetConn.WriteTo(b, p.remote)
}

func (p *writeOnlyConn) Close() error {
	return nil
}

func (p *writeOnlyConn) LocalAddr() net.Addr {
	return p.packetConn.LocalAddr()
}

func (p *writeOnlyConn) RemoteAddr() net.Addr {
	return p.remote
}

func (p *writeOnlyConn) SetDeadline(t time.Time) error {
	return nil
}

func (p *writeOnlyConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (p *writeOnlyConn) SetWriteDeadline(t time.Time) error {
	return nil
}
