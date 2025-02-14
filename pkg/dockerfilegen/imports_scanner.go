package dockerfilegen

import (
	"log"

	"github.com/openshift-knative/hack/pkg/dockerfilegen/imports"
	"k8s.io/apimachinery/pkg/util/sets"
)

func scanImports(paths sets.Set[string], rootDir string, packages []string, tags []string) error {
	m, err := imports.ScanForMains(rootDir, packages, tags)
	if err != nil {
		return err
	}
	for _, pkg := range m.UnsortedList() {
		if !paths.Has(pkg) && !paths.Has("vendor/"+pkg) {
			paths.Insert(pkg)
			log.Println("Found main package from imports:", pkg)
		}
	}

	return nil
}
