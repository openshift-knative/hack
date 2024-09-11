package main

import (
	"flag"
	"log"

	"github.com/openshift-knative/hack/pkg/maven"
)

const (
	pomFileFlag = "path"
)

func main() {
	metadata, err := maven.ScrapRedHatMavenRegistry(maven.RedHatMavenGA)
	if err != nil {
		log.Fatal(err)
	}

	path := flag.String(pomFileFlag, "", "POM file path")
	flag.Parse()

	if path == nil || *path == "" {
		log.Fatalf("--%s flag is required", pomFileFlag)
	}

	if err := maven.UpdatePomFile(metadata, maven.RedHatMavenGA, *path); err != nil {
		log.Fatal(err)
	}
}
