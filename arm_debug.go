// +build mdp_debug

package mdp

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const debug = true

func init() {
	log.Logger = log.Logger.Level(zerolog.DebugLevel).Output(zerolog.NewConsoleWriter())
}
