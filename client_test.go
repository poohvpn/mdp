package mdp

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient(tt *testing.T) {
	t := require.New(tt)
	client, err := NewClient(Config{
		IP4:     net.IPv4(127, 0, 0, 1),
		Port:    1989,
		Threads: 0,
	})
	t.NoError(err)
	local := client.LocalAddr()
	remote := client.RemoteAddr()
	t.Equal("mdp", local.Network())
	t.True(strings.HasPrefix(local.String(), "127.0.0.1:"))
	t.Equal("mdp", remote.Network())
	t.Equal("127.0.0.1:1989", remote.String())

	fmt.Println("client SessionID:", client.SessionID())
	t.NoError(client.Close())
}
