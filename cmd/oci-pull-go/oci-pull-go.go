package main

import (
	"os"

	"github.com/coffeemakingtoaster/oci-pull-go/pkg/pull"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout}).With().Caller().Logger()

	pull.PullToPath(os.Args[1], "./download.tar")
}
