package imports_test

import (
	"path"
	"runtime"
	"testing"

	"github.com/openshift-knative/hack/pkg/dockerfilegen/imports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanForMains(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	rootDir := path.Dir(path.Dir(path.Dir(path.Dir(file))))
	pkgs, err := imports.ScanForMains(rootDir,
		[]string{"pkg/dockerfilegen/imports/testdata"},
		[]string{"tools"},
	)
	require.NoError(t, err)
	assert.True(t, pkgs.Has("github.com/openshift-knative/hack/cmd/prowgen"))
	assert.True(t, pkgs.Has("knative.dev/pkg/codegen/cmd/injection-gen"))
	assert.Equal(t, 2, pkgs.Len())
}
