package mdp

import (
	"math/rand"
	"net"
	"reflect"
	"time"

	"github.com/poohvpn/icmdp"
	"github.com/rs/zerolog/log"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

const (
	idSize     = 4
	natTimeout = 30 * time.Second
	queueSize  = 1024
)

func endpointIndex(conn net.Conn) uint64 {
	index := uint64(0)
	local := conn.LocalAddr()
	remote := conn.RemoteAddr()
	network := remote.Network()
	switch network {
	case "tcp":
		index = uint64(0x6)
	case "udp":
		index = uint64(0x11)
	case "icmdp":
		index = uint64(0x1)
	default:
		log.Panic().Str("network", network).Msg("unknown network")
	}
	index <<= 1
	switch v := local.(type) {
	case *net.TCPAddr:
		index += uint64(v.Port)
	case *net.UDPAddr:
		index += uint64(v.Port)
	case *icmdp.Addr:
		index += uint64(v.Seq)
	default:
		log.Panic().Str("local.type", reflect.TypeOf(local).String()).Msg("unknown local address type")
	}
	index <<= 16
	switch v := remote.(type) {
	case *net.TCPAddr:
		index += uint64(v.Port)
	case *net.UDPAddr:
		index += uint64(v.Port)
	case *icmdp.Addr:
		index += uint64(v.ID) // mainly for icmdp server side to represent client port
	default:
		log.Panic().Str("remote.type", reflect.TypeOf(remote).String()).Msg("unknown remote address type")
	}
	return index
}

type rawPacket struct {
	Addr *Addr
	Data []byte
}

type Obfuscator interface {
	ObfuPacketConn(conn net.PacketConn) net.PacketConn
	ObfuStreamConn(conn net.Conn) net.Conn
	ObfuDatagramConn(conn net.Conn) net.Conn
}

type nopObfuscator struct{}

var _ Obfuscator = nopObfuscator{}

func (o nopObfuscator) ObfuPacketConn(conn net.PacketConn) net.PacketConn {
	return conn
}

func (o nopObfuscator) ObfuDatagramConn(conn net.Conn) net.Conn {
	return conn
}

func (o nopObfuscator) ObfuStreamConn(conn net.Conn) net.Conn {
	return conn
}
