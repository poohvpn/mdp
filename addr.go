package mdp

import (
	"net"
	"reflect"

	"github.com/poohvpn/icmdp"
	"github.com/rs/zerolog/log"
)

type Addr struct {
	IP   net.IP
	Port int
	Zone string
	sess *session
}

func (a *Addr) Network() string { return "mdp" }

func (a *Addr) String() string {
	return (&net.UDPAddr{
		IP:   a.IP,
		Port: a.Port,
		Zone: a.Zone,
	}).String()
}

func (a *Addr) ID() uint32 {
	return a.sess.id
}

func fromNetAddr(netAddr net.Addr) *Addr {
	addr := new(Addr)
	switch a := netAddr.(type) {
	case *net.TCPAddr:
		addr.IP = a.IP
		addr.Port = a.Port
		addr.Zone = a.Zone
	case *net.UDPAddr:
		addr.IP = a.IP
		addr.Port = a.Port
		addr.Zone = a.Zone
	case *icmdp.Addr:
		addr.IP = a.IP
		addr.Port = int(a.ID)
		addr.Zone = a.Zone
	case *Addr:
		addr.IP = a.IP
		addr.Port = a.Port
		addr.Zone = a.Zone
	default:
		log.Panic().Str("addr.(type)", reflect.TypeOf(netAddr).String()).Msg("unknown addr type")
	}
	return addr
}
