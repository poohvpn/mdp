package mdp

import (
	"encoding/binary"
	"math/rand"
	"net"
	"reflect"
	"time"

	"github.com/poohvpn/icmdp"
	"github.com/poohvpn/pooh"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

const (
	sessionIDSize = 4
	nodeIDSize    = 4
	natTimeout    = 30 * time.Second
	queueSize     = 1024
)

type inputPacket struct {
	Addr *Addr
	Data []byte
}

type Obfuscator interface {
	ObfuscatePacketConn(conn net.PacketConn) net.PacketConn
	ObfuscateStreamConn(conn net.Conn) net.Conn
	ObfuscateDatagramConn(conn net.Conn) net.Conn
}

type nopObfuscator struct{}

var _ Obfuscator = nopObfuscator{}

func (o nopObfuscator) ObfuscatePacketConn(conn net.PacketConn) net.PacketConn {
	return conn
}

func (o nopObfuscator) ObfuscateDatagramConn(conn net.Conn) net.Conn {
	return conn
}

func (o nopObfuscator) ObfuscateStreamConn(conn net.Conn) net.Conn {
	return conn
}

func readStreamIDs(conn pooh.Conn) (sid, nid uint32, err error) {
	sid, err = conn.Uint32()
	if err != nil {
		return
	}
	nid, err = conn.Uint32()
	if err != nil {
		return
	}
	return
}

func connIndex(conn net.Conn) uint64 {
	switch local := conn.LocalAddr().(type) {
	case *net.TCPAddr:
		remote := conn.RemoteAddr().(*net.TCPAddr)
		return endpointIndex(
			pooh.IsIPv4(local.IP),
			endpointTCP,
			uint16(local.Port),
			uint16(remote.Port),
		)
	case *net.UDPAddr:
		remote := conn.RemoteAddr().(*net.UDPAddr)
		return endpointIndex(
			pooh.IsIPv4(local.IP),
			endpointUDP,
			uint16(local.Port),
			uint16(remote.Port),
		)
	case *icmdp.Addr:
		remote := conn.RemoteAddr().(*icmdp.Addr)
		return endpointIndex(
			pooh.IsIPv4(local.IP),
			endpointICMDP,
			local.Seq,
			remote.ID, // mainly for icmdp server side to represent client port
		)
	default:
		panic(reflect.TypeOf(local).String())
	}
}

func forwardIndex(sid, nid uint32) uint64 {
	return uint64(sid)<<32 + uint64(nid)
}

func readPacketIDs(p []byte) (sid, nid uint32, data []byte) {
	pLen := len(p)
	return binary.BigEndian.Uint32(p[pLen-4:]), binary.BigEndian.Uint32(p[pLen-8:]), data[:pLen-8]
}

func endpointIndex(ipv4 bool, typ endpointType, localPort, remotePort uint16) uint64 {
	i := uint64(4)
	if !ipv4 {
		i = 6
	}
	i <<= 8
	i += uint64(typ)
	i <<= 16
	i += uint64(localPort)
	i <<= 16
	i += uint64(remotePort)
	return i
}
