package deployer

import (
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/kairos-io/AuroraBoot/internal/log"
	"github.com/kairos-io/AuroraBoot/pkg/schema"

	"github.com/spectrocloud-labs/herd"
	"gopkg.in/yaml.v3"
)

type Deployer struct {
	*herd.Graph
	Config   schema.Config
	Artifact schema.ReleaseArtifact
}

func NewDeployer(c schema.Config, a schema.ReleaseArtifact, opts ...herd.GraphOption) *Deployer {
	d := &Deployer{Config: c, Artifact: a}
	d.Graph = herd.DAG(opts...)

	return d
}

func (d *Deployer) CollectErrors() error {
	var err error
	for _, layer := range d.Analyze() {
		for _, op := range layer {
			if op.Error != nil {
				err = multierror.Append(err, op.Error)
			}
		}
	}

	return err
}

func (d *Deployer) WriteDag() {
	graph := d.Analyze()
	for i, layer := range graph {
		log.Log.Printf("%d.", (i + 1))
		for _, op := range layer {
			if !op.Ignored {
				if op.Error != nil {
					log.Log.Printf(" <%s> (error: %s) (background: %t)", op.Name, op.Error.Error(), op.Background)
				} else {
					log.Log.Printf(" <%s> (background: %t)", op.Name, op.Background)
				}
			}
		}
		log.Log.Print("")
	}
}

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
