package deployer

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/kairos-io/AuroraBoot/pkg/schema"

	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"gopkg.in/yaml.v3"
)

func LoadByte(b []byte) (*schema.Config, *schema.ReleaseArtifact, error) {
	config := &schema.Config{}
	release := &schema.ReleaseArtifact{}

	if err := yaml.Unmarshal(b, config); err != nil {
		return nil, nil, err
	}

	if err := yaml.Unmarshal(b, release); err != nil {
		return nil, nil, err
	}

	return config, release, nil
}

// LoadFile loads a configuration file and returns the AuroraBoot configuration
// and release artifact information
func LoadFile(file string) (*schema.Config, *schema.ReleaseArtifact, error) {

	dat, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, err
	}

	return LoadByte(dat)
}

// Start starts the auroraboot deployer
func Start(config *schema.Config, release *schema.ReleaseArtifact) error {

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

	Register(g, *release, *config, f.Name())

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
				log.Printf(" <%s> (background: %t enabled: %t)", op.Name, op.Background)
			}
		}
		log.Print("")
	}
}
