package mdp

import (
	"errors"
	"net"
	"time"
)

var _ net.Conn = &Client{}

func NewClient(config Config) (*Client, error) {
	c := &Client{
		sess: newSession(config).addForwardEndpoints(),
	}
	return c, nil
}

type Client struct {
	sess *session
}

func (c *Client) Read(b []byte) (n int, err error) {
	select {
	case <-c.sess.closeOnce.Wait():
		return 0, errors.New("mdp: client is closed")
	case packet := <-c.sess.dstInputCh:
		n = copy(b, packet.Data)
		return
	}
}

func (c *Client) Write(b []byte) (n int, err error) {
	return len(b), c.sess.output(b,true)
}

func (c *Client) Close() error {
	return c.sess.Close()
}

func (c *Client) LocalAddr() net.Addr {
	return c.sess.srcAddr
}

func (c *Client) RemoteAddr() net.Addr {
	return c.sess.dstAddr
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
