package main

import (
	"github.com/poohvpn/mdp"
	"github.com/rs/zerolog/log"
)

func main() {
	server, err := mdp.Listen(1989)
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 65546)
	for {
		nr, addr, err := server.ReadFrom(buf)
		if err != nil {
			panic(err)
		}
		log.Info().
			Str("addr", addr.String()).
			Uint32("id", addr.(*mdp.Addr).ID()).
			Bytes("data", buf[:nr]).
			Msg("ReadFrom")
		nw, err := server.WriteTo(buf[:nr], addr)
		if nw != nr {
			panic("nw != nr")
		}
		if err != nil {
			panic(err)
		}
	}
}
