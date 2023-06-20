package main

import (
	"log"

	"github.com/openshift-knative/hack/pkg/prowcopy"
)

func main() {
	if err := prowcopy.Main(); err != nil {
		log.Fatal(err)
	}
}
