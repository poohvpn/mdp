package mdp

import (
	"errors"
	"net"
	"time"

	"github.com/poohvpn/pooh"
	"github.com/rs/zerolog/log"
)

var _ net.Conn = &Client{}

type ClientConfig struct {
	IP4          net.IP
	IP6          net.IP
	Port         int
	Zone         string
	Threads      int
	DisableICMDP bool
	DisableTCP   bool
	DisableUDP   bool
	Obfuscator   Obfuscator
}

func (c *ClientConfig) fix() {
	if c.Threads <= 0 {
		c.Threads = 1
	} else if c.Threads > 32 {
		c.Threads = 32
	}
	if c.Obfuscator == nil {
		c.Obfuscator = nopObfuscator{}
	}
}

func NewClient(config ClientConfig) (*Client, error) {
	config.fix()
	c := &Client{
		config: config,
		sess:   newSession(),
	}
	err := c.tryDialAll()
	if err != nil {
		return nil, err
	}
	return c, nil
}

type Client struct {
	config ClientConfig

	sess *session
}

func (c *Client) tryDialAll() error {
	networks := make([]string, 0, 3)
	if c.config.DisableUDP &&
		c.config.DisableTCP &&
		c.config.DisableICMDP {
		return errors.New("mdp: no available network to use")
	}
	if !c.config.DisableUDP {
		networks = append(networks, "udp")
	}
	if !c.config.DisableTCP {
		networks = append(networks, "tcp")
	}
	if !c.config.DisableICMDP {
		networks = append(networks, "icmdp")
	}

	var (
		success bool
		lastErr error
	)
	if c.config.IP4 != nil {
		addr := &Addr{
			IP:   c.config.IP4,
			Port: c.config.Port,
		}
		for _, network := range networks {
			network += "4"
			err := c.sess.dial(network, addr, c.config.Obfuscator)
			if err != nil {
				log.Warn().Err(err).Str("network", network).Msg("mdp: dial")
				lastErr = err
				continue
			}
			success = true
		}
	}
	if c.config.IP6 != nil {
		addr := &Addr{
			IP:   c.config.IP6,
			Port: c.config.Port,
			Zone: c.config.Zone,
		}
		for _, network := range networks {
			network += "6"
			err := c.sess.dial(network, addr, c.config.Obfuscator)
			if err != nil {
				log.Warn().Err(err).Str("network", network).Msg("mdp: dial")
				lastErr = err
				continue
			}
			success = true
		}
	}
	if !success {
		return lastErr
	}
	return nil
}

func (c *Client) Read(b []byte) (n int, err error) {
	select {
	case <-c.sess.closeOnce.Wait():
		return 0, errors.New("mdp: client is closed")
	case packet := <-c.sess.packetCh:
		n = copy(b, packet.Data)
		return
	}
}

func (c *Client) Write(b []byte) (n int, err error) {
	return len(b), c.sess.send(b)
}

func (c *Client) Close() error {
	return c.sess.Close()
}

func (c *Client) LocalAddr() net.Addr {
	return c.sess.localAddr
}

func (c *Client) RemoteAddr() net.Addr {
	return c.sess.remoteAddr
}

func (c *Client) SetDeadline(t time.Time) error {
	panic("implement me")
}

func (c *Client) SetReadDeadline(t time.Time) error {
	panic("implement me")
}

func (c *Client) SetWriteDeadline(t time.Time) error {
	panic("implement me")
}

func (c *Client) ID() uint32 {
	return c.sess.id
}

type clientDatagramConn struct {
	net.Conn
	id uint32
}

func (c *clientDatagramConn) Write(p []byte) (n int, err error) {
	n = len(p)
	_, err = c.Conn.Write(append(p, pooh.Uint322Bytes(c.id)...))
	return
}
