package deployer

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"gopkg.in/yaml.v3"
)

func Start(file string) error {
	fmt.Println("Reading ", file)
	config := &Config{}
	release := &ReleaseArtifact{}

	dat, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(dat, config); err != nil {
		return err
	}

	if err := yaml.Unmarshal(dat, release); err != nil {
		return err
	}

	f, err := ioutil.TempFile("", "auroraboot-dat")
	if err != nil {
		return err
	}

	_, err = f.WriteString(config.CloudConfig)
	if err != nil {
		return err
	}

	// Have a dag for our ops
	g := herd.DAG(herd.CollectOrphans)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Register what to do!
	if err := RegisterNetbootOperations(g, *release, f.Name()); err != nil {
		return err
	}

	if err := RegisterISOOperations(g, *release, f.Name()); err != nil {
		return err
	}

	writeDag(g.Analyze())

	ctx := context.Background()
	return g.Run(ctx)
}

func writeDag(d [][]herd.GraphEntry) {
	for i, layer := range d {
		log.Printf("%d.", (i + 1))
		for _, op := range layer {
			if op.Error != nil {
				log.Printf(" <%s> (error: %s) (background: %t)", op.Name, op.Error.Error(), op.Background)
			} else {
				log.Printf(" <%s> (background: %t)", op.Name, op.Background)
			}
		}
		log.Print("")
	}
}
