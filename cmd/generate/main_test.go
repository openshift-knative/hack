package main_test

import (
	"os"
	"os/exec"
	"path"
	"runtime"
	"testing"

	main "github.com/openshift-knative/hack/cmd/generate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateMain(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := path.Dir(path.Dir(path.Dir(here)))
	wantOutputPath := path.Join(root, "pkg", "project", "testoutput")
	tmpPath := t.TempDir()
	err := os.Chdir(tmpPath)
	require.NoError(t, err)

	err = main.GenerateMain([]string{
		"--root-dir", root,
		"--generators", "dockerfile",
		"--project-file", "pkg/project/testdata/project.yaml",
		"--includes", "^cmd/.*discover.*",
		"--additional-packages", "tzdata",
		"--additional-packages", "rsync",
		"--images-from", "hack",
		"--images-from-url-format", "https://raw.githubusercontent.com/openshift-knative/%s/%s/pkg/project/testdata/additional-images.yaml",
		"--output", path.Join(tmpPath, "openshift"),
	})
	require.NoError(t, err)
	command := "diff --unified -r " + wantOutputPath + " " + tmpPath

	outb, err := exec.Command("sh", "-c", command).Output()
	out := ""
	if outb != nil {
		out = string(outb)
	}
	require.NoError(t, err, "Output: ", out)
	assert.Equal(t, "", out)
}
