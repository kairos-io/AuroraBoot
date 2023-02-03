package deployer

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"gopkg.in/yaml.v3"
)

func LoadFile(file string) (*Config, *ReleaseArtifact, error) {
	config := &Config{}
	release := &ReleaseArtifact{}

	dat, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, err
	}

	if err := yaml.Unmarshal(dat, config); err != nil {
		return nil, nil, err
	}

	if err := yaml.Unmarshal(dat, release); err != nil {
		return nil, nil, err
	}

	return config, release, nil
}

func Start(config *Config, release *ReleaseArtifact) error {

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

	if !config.DisableNetboot {
		// Register what to do!
		if err := RegisterNetbootOperations(g, *release, *config, f.Name()); err != nil {
			return err
		}
	}

	if !config.DisableISOboot {
		if err := RegisterISOOperations(g, *release, *config, f.Name()); err != nil {
			return err
		}
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
