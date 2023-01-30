package main

import (
	"context"
	"fmt"

	"github.com/kairos-io/AuroraBoot/internal/deployer"
	"github.com/spectrocloud-labs/herd"
)

func main() {

	// Have a dag for our ops
	g := herd.DAG()

	if err := deployer.RegisterOperations(g, deployer.ReleaseArtifact{
		ArtifactVersion: "v1.5.0",
		ReleaseVersion:  "v1.5.0",
		Flavor:          "rockylinux",
		Repository:      "kairos-io/kairos",
	}); err != nil {
		panic(err)
	}

	for i, layer := range g.Analyze() {
		fmt.Printf("%d.", (i + 1))
		for _, op := range layer {
			fmt.Printf(" <%s> (background: %t)", op.Name, op.Background)
		}
		fmt.Println("")
	}

	g.Run(context.Background())
}
