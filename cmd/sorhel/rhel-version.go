package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/openshift-knative/hack/pkg/rhel"
)

func main() {
	soBranch := flag.String("so-branch", "", "Serverless Operator Branch name")
	flag.Parse()

	v, err := rhel.ForSOBranchName(*soBranch)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(v)
}
