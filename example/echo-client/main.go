package main

import (
	"net"
	"time"

	"github.com/poohvpn/mdp"
	"github.com/rs/zerolog/log"
)

func main() {
	client, err := mdp.NewClient(mdp.Config{
		IP4:          net.IPv4(127, 0, 0, 1),
		Port:         1989,
		Threads:      1,
		DisableTCP:   true,
		DisableICMDP: true,
	})
	if err != nil {
		panic(err)
	}
	log.Info().
		Str("local", client.LocalAddr().String()).
		Str("remote", client.RemoteAddr().String()).
		Uint32("id", client.SessionID()).
		Msg("Client")
	buf := make([]byte, 65536)
	msg := []byte("hello world")
	for {
		log.Info().Uint32("id", client.SessionID()).Bytes("data", msg).Msg("Write")
		nw, err := client.Write(msg)
		if err != nil {
			panic(err)
		}
		if nw != len(msg) {
			panic("nw != len(msg)")
		}
		log.Info().Uint32("id", client.SessionID()).Msg("Read")
		nr, err := client.Read(buf)
		if err != nil {
			panic(err)
		}
		if nr != nw {
			panic("nr != nw")
		}
		time.Sleep(time.Second)
	}
}
