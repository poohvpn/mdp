package mdp

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAddr_Network(t *testing.T) {
	assert.Equal(t, "mdp", (&Addr{}).Network())
}

func TestAddr_String(tt *testing.T) {
	t := assert.New(tt)
	t.Equal(":123", (&Addr{
		IP:   nil,
		Port: 123,
		Zone: "",
	}).String())
}
