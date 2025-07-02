package main

import (
	"os"

	"github.com/coffeemakingtoaster/oci-pull-go/pkg/registry"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout}).With().Caller().Logger()

	registry.DownloadOciToPath("https://ghcr.io/v2", os.Args[1], "./download.tar")
}
