package mdp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForwardIndex(tt *testing.T) {
	t := assert.New(tt)
	t.Equal(uint64(0x100000002),forwardIndex(0x1,0x2))
}