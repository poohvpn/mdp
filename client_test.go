package mdp

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"net"
	"strings"
	"testing"
)

func TestClient(tt *testing.T) {
	t := require.New(tt)
	client, err := NewClient(ClientConfig{
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

	fmt.Println("client ID:", client.ID())
	t.NoError(client.Close())
}
