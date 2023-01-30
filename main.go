package main

import (
	"context"
	"fmt"

	"github.com/kairos-io/AuroraBoot/internal/deployer"
	"github.com/spectrocloud-labs/herd"
)

func main() {

	// Have a dag for our ops
	g := herd.DAG(herd.EnableInit)

	if err := deployer.RegisterOperations(g, deployer.ReleaseArtifact{
		ArtifactVersion: "",
		ReleaseVersion:  "",
		Flavor:          "",
		Repository:      "",
	}); err != nil {
		panic(err)
	}

	fmt.Println(g.Analyze())

	g.Run(context.Background())
}
