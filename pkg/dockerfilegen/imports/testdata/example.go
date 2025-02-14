//go:build tools

package testdata

import (
	_ "github.com/openshift-knative/hack/cmd/prowgen"
	_ "knative.dev/pkg"
	_ "knative.dev/pkg/codegen/cmd/injection-gen"
)
