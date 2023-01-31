package main

import (
	"os"

	"github.com/kairos-io/AuroraBoot/deployer"
)

func main() {

	if err := deployer.Start(os.Args[1]); err != nil {
		panic(err)
	}

}
