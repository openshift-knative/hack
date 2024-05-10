package main

import (
	"os"

	"github.com/openshift-knative/hack/internal/deviate"
)

var (
	ExitFunc = os.Exit        //nolint:gochecknoglobals
	Opts     []deviate.Option //nolint:gochecknoglobals
)

func main() {
	ExitFunc(deviate.Main(Opts...))
}

func Main() {
	main()
}
