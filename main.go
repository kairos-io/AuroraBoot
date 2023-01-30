package main

import (
	"context"
	"os"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
)

func main() {

	// Have a dag for our ops
	g := herd.DAG()
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	if err := deployer.RegisterNetbootOperations(g, deployer.ReleaseArtifact{
		ArtifactVersion: "v1.5.0",
		ReleaseVersion:  "v1.5.0",
		Flavor:          "rockylinux",
		Repository:      "kairos-io/kairos",
	}); err != nil {
		panic(err)
	}

	for i, layer := range g.Analyze() {
		log.Printf("%d.", (i + 1))
		for _, op := range layer {
			log.Printf(" <%s> (background: %t)", op.Name, op.Background)
		}
	}

	g.Run(context.Background())

	for i, layer := range g.Analyze() {
		log.Printf("%d.", (i + 1))
		for _, op := range layer {
			if op.Error != nil {
				log.Printf(" <%s> (error: %s)", op.Name, op.Error.Error())
			}
		}
		log.Print("")
	}

}
