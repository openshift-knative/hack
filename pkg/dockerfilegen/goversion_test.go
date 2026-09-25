package dockerfilegen

import (
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/openshift-knative/hack/pkg/project"
)

func Test_componentBranchForTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{
			name: "knative-nightly maps to release-next",
			tag:  "knative-nightly",
			want: "release-next",
		},
		{
			name: "versioned tag strips knative- prefix",
			tag:  "knative-v1.17",
			want: "release-v1.17",
		},
		{
			name: "tag without knative prefix",
			tag:  "v1.11",
			want: "release-v1.11",
		},
		{
			name: "main tag",
			tag:  "main",
			want: "release-main",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := componentBranchForTag(tt.tag); got != tt.want {
				t.Errorf("componentBranchForTag(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func Test_soBranchForMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata *project.Metadata
		want     string
	}{
		{
			name:     "knative-nightly tag returns main",
			metadata: &project.Metadata{Project: project.Project{Tag: "knative-nightly"}},
			want:     "main",
		},
		{
			name:     "main tag returns main",
			metadata: &project.Metadata{Project: project.Project{Tag: "main"}},
			want:     "main",
		},
		{
			name:     "upstream v1.17 tag maps to release-1.37",
			metadata: &project.Metadata{Project: project.Project{Tag: "knative-v1.17"}},
			want:     "release-1.37",
		},
		{
			name:     "upstream v1.21 tag maps to release-1.38",
			metadata: &project.Metadata{Project: project.Project{Tag: "knative-v1.21"}},
			want:     "release-1.38",
		},
		{
			name:     "SO version 1.37.0 maps to release-1.37",
			metadata: &project.Metadata{Project: project.Project{Version: "1.37.0"}},
			want:     "release-1.37",
		},
		{
			name:     "SO patch version 1.37.1 maps to release-1.37",
			metadata: &project.Metadata{Project: project.Project{Version: "1.37.1"}},
			want:     "release-1.37",
		},
		{
			name:     "SO version 1.38.0 maps to release-1.38",
			metadata: &project.Metadata{Project: project.Project{Version: "1.38.0"}},
			want:     "release-1.38",
		},
		{
			name:     "invalid version returns empty",
			metadata: &project.Metadata{Project: project.Project{Version: "not-a-version"}},
			want:     "",
		},
		{
			name:     "empty metadata returns empty",
			metadata: &project.Metadata{},
			want:     "",
		},
		{
			name:     "tag takes precedence over version",
			metadata: &project.Metadata{Project: project.Project{Tag: "knative-nightly", Version: "1.37.0"}},
			want:     "main",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := soBranchForMetadata(tt.metadata); got != tt.want {
				t.Errorf("soBranchForMetadata() = %q, want %q", got, tt.want)
			}
		})
	}
}

func makeConfigYAML(imagePrefix, branch, golangVersion string) []byte {
	tmpl := `config:
  branches:
    %s:
      golangVersion: "%s"
repositories:
- imagePrefix: %s
`
	return []byte(fmt.Sprintf(tmpl, branch, golangVersion, imagePrefix))
}

func makeConfigYAMLNoGolang(imagePrefix, branch string) []byte {
	tmpl := `config:
  branches:
    %s: {}
repositories:
- imagePrefix: %s
`
	return []byte(fmt.Sprintf(tmpl, branch, imagePrefix))
}

func Test_golangVersionFromRepoConfig(t *testing.T) {
	tests := []struct {
		name        string
		fs          fstest.MapFS
		imagePrefix string
		branch      string
		wantVersion string
		wantNil     bool
	}{
		{
			name: "returns golang version for matching repo and branch",
			fs: fstest.MapFS{
				"eventing.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-eventing", "release-v1.17", "1.23"),
				},
			},
			imagePrefix: "knative-eventing",
			branch:      "release-v1.17",
			wantVersion: "1.23",
		},
		{
			name: "returns nil when branch not found",
			fs: fstest.MapFS{
				"eventing.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-eventing", "release-v1.17", "1.23"),
				},
			},
			imagePrefix: "knative-eventing",
			branch:      "release-v1.16",
			wantNil:     true,
		},
		{
			name: "returns nil when imagePrefix not found",
			fs: fstest.MapFS{
				"eventing.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-eventing", "release-v1.17", "1.23"),
				},
			},
			imagePrefix: "knative-serving",
			branch:      "release-v1.17",
			wantNil:     true,
		},
		{
			name: "returns nil when golangVersion not set in branch config",
			fs: fstest.MapFS{
				"eventing.yaml": &fstest.MapFile{
					Data: makeConfigYAMLNoGolang("knative-eventing", "release-v1.17"),
				},
			},
			imagePrefix: "knative-eventing",
			branch:      "release-v1.17",
			wantNil:     true,
		},
		{
			name: "scans multiple config files and finds match in second",
			fs: fstest.MapFS{
				"aaa-eventing.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-eventing", "release-v1.17", "1.22"),
				},
				"bbb-serving.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-serving", "release-v1.17", "1.23"),
				},
			},
			imagePrefix: "knative-serving",
			branch:      "release-v1.17",
			wantVersion: "1.23",
		},
		{
			name: "skips directories",
			fs: fstest.MapFS{
				"subdir": &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
				"eventing.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-eventing", "release-v1.17", "1.23"),
				},
			},
			imagePrefix: "knative-eventing",
			branch:      "release-v1.17",
			wantVersion: "1.23",
		},
		{
			name: "returns golang version for main branch",
			fs: fstest.MapFS{
				"eventing.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-eventing", "main", "1.25"),
				},
			},
			imagePrefix: "knative-eventing",
			branch:      "main",
			wantVersion: "1.25",
		},
		{
			name: "returns golang version for release-1.38 branch",
			fs: fstest.MapFS{
				"serving.yaml": &fstest.MapFile{
					Data: makeConfigYAML("knative-serving", "release-1.38", "1.24"),
				},
			},
			imagePrefix: "knative-serving",
			branch:      "release-1.38",
			wantVersion: "1.24",
		},
		{
			name:        "returns nil for empty filesystem",
			fs:          fstest.MapFS{},
			imagePrefix: "knative-eventing",
			branch:      "release-v1.17",
			wantNil:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := golangVersionFromRepoConfig(tt.fs, tt.imagePrefix, tt.branch)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %q", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result, got nil")
			}
			if *got != tt.wantVersion {
				t.Errorf("got %q, want %q", *got, tt.wantVersion)
			}
		})
	}
}

func Test_golangVersionFromSOConfig(t *testing.T) {
	soConfigWithVersion := func(branch, version string) fstest.MapFS {
		yaml := fmt.Sprintf(`config:
  branches:
    %s:
      golangVersion: "%s"
repositories:
- imagePrefix: serverless
`, branch, version)
		return fstest.MapFS{
			"serverless-operator.yaml": &fstest.MapFile{Data: []byte(yaml)},
		}
	}

	soConfigNoBranch := fstest.MapFS{
		"serverless-operator.yaml": &fstest.MapFile{
			Data: []byte(`config:
  branches:
    main:
      golangVersion: "1.25"
repositories:
- imagePrefix: serverless
`),
		},
	}

	soConfigNoGolang := fstest.MapFS{
		"serverless-operator.yaml": &fstest.MapFile{
			Data: []byte(`config:
  branches:
    release-1.37: {}
repositories:
- imagePrefix: serverless
`),
		},
	}

	tests := []struct {
		name        string
		fs          fstest.MapFS
		branch      string
		wantVersion string
		wantNil     bool
		wantErr     bool
	}{
		{
			name:        "returns golang version for matching branch",
			fs:          soConfigWithVersion("release-1.37", "1.23"),
			branch:      "release-1.37",
			wantVersion: "1.23",
		},
		{
			name:        "returns golang version for main",
			fs:          soConfigWithVersion("main", "1.25"),
			branch:      "main",
			wantVersion: "1.25",
		},
		{
			name:        "returns golang version for release-1.38",
			fs:          soConfigWithVersion("release-1.38", "1.24"),
			branch:      "release-1.38",
			wantVersion: "1.24",
		},
		{
			name:    "returns nil when branch not found",
			fs:      soConfigNoBranch,
			branch:  "release-1.99",
			wantNil: true,
		},
		{
			name:    "returns nil when golangVersion not set",
			fs:      soConfigNoGolang,
			branch:  "release-1.37",
			wantNil: true,
		},
		{
			name:    "returns error when SO config file missing",
			fs:      fstest.MapFS{},
			branch:  "release-1.37",
			wantErr: true,
		},
		{
			name: "returns error when SO config is malformed YAML",
			fs: fstest.MapFS{
				"serverless-operator.yaml": &fstest.MapFile{Data: []byte(`not: valid: yaml: [`)},
			},
			branch:  "main",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := golangVersionFromSOConfig(tt.fs, tt.branch)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %q", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result, got nil")
			}
			if *got != tt.wantVersion {
				t.Errorf("got %q, want %q", *got, tt.wantVersion)
			}
		})
	}
}

func Test_goVersionFromConfig(t *testing.T) {
	repoAndSOConfig := func(imagePrefix, repoBranch, repoGoVer, soBranch, soGoVer string) fstest.MapFS {
		repoYaml := fmt.Sprintf(`config:
  branches:
    %s:
      golangVersion: "%s"
repositories:
- imagePrefix: %s
`, repoBranch, repoGoVer, imagePrefix)
		soYaml := fmt.Sprintf(`config:
  branches:
    %s:
      golangVersion: "%s"
repositories:
- imagePrefix: serverless
`, soBranch, soGoVer)
		return fstest.MapFS{
			"eventing.yaml":            &fstest.MapFile{Data: []byte(repoYaml)},
			"serverless-operator.yaml": &fstest.MapFile{Data: []byte(soYaml)},
		}
	}

	soOnlyConfig := func(soBranch, soGoVer string) fstest.MapFS {
		soYaml := fmt.Sprintf(`config:
  branches:
    %s:
      golangVersion: "%s"
repositories:
- imagePrefix: serverless
`, soBranch, soGoVer)
		return fstest.MapFS{
			"serverless-operator.yaml": &fstest.MapFile{Data: []byte(soYaml)},
		}
	}

	tests := []struct {
		name        string
		fs          fstest.MapFS
		metadata    *project.Metadata
		wantVersion string
		wantNil     bool
		wantErr     bool
	}{
		{
			name: "component repo: returns repo-specific golang version",
			fs:   repoAndSOConfig("knative-eventing", "release-v1.17", "1.22", "release-1.37", "1.25"),
			metadata: &project.Metadata{
				Project: project.Project{
					Tag:         "knative-v1.17",
					ImagePrefix: "knative-eventing",
				},
			},
			wantVersion: "1.22",
		},
		{
			name: "component repo: falls back to SO when repo has no golang version",
			fs: func() fstest.MapFS {
				repoYaml := `config:
  branches:
    release-v1.17: {}
repositories:
- imagePrefix: knative-eventing
`
				soYaml := `config:
  branches:
    release-1.37:
      golangVersion: "1.25"
repositories:
- imagePrefix: serverless
`
				return fstest.MapFS{
					"eventing.yaml":            &fstest.MapFile{Data: []byte(repoYaml)},
					"serverless-operator.yaml": &fstest.MapFile{Data: []byte(soYaml)},
				}
			}(),
			metadata: &project.Metadata{
				Project: project.Project{
					Tag:         "knative-v1.17",
					ImagePrefix: "knative-eventing",
				},
			},
			wantVersion: "1.25",
		},
		{
			name: "SO version: returns golang version for release branch",
			fs:   soOnlyConfig("release-1.37", "1.25"),
			metadata: &project.Metadata{
				Project: project.Project{Version: "1.37.0"},
			},
			wantVersion: "1.25",
		},
		{
			name: "SO patch version: returns golang version (original bug fix)",
			fs:   soOnlyConfig("release-1.37", "1.25"),
			metadata: &project.Metadata{
				Project: project.Project{Version: "1.37.1"},
			},
			wantVersion: "1.25",
		},
		{
			name: "SO patch version: 1.38.2 maps to release-1.38",
			fs:   soOnlyConfig("release-1.38", "1.25"),
			metadata: &project.Metadata{
				Project: project.Project{Version: "1.38.2"},
			},
			wantVersion: "1.25",
		},
		{
			name: "knative-nightly component: falls back to SO main",
			fs:   soOnlyConfig("main", "1.25"),
			metadata: &project.Metadata{
				Project: project.Project{
					Tag:         "knative-nightly",
					ImagePrefix: "knative-eventing",
				},
			},
			wantVersion: "1.25",
		},
		{
			name:     "empty metadata returns nil",
			fs:       fstest.MapFS{"serverless-operator.yaml": &fstest.MapFile{Data: []byte(`config: {}`)}},
			metadata: &project.Metadata{},
			wantNil:  true,
		},
		{
			name: "no imagePrefix, no version, no tag returns nil",
			fs:   fstest.MapFS{"serverless-operator.yaml": &fstest.MapFile{Data: []byte(`config: {}`)}},
			metadata: &project.Metadata{
				Project: project.Project{},
			},
			wantNil: true,
		},
		{
			name: "component repo: propagates repo config error",
			fs: fstest.MapFS{
				"eventing.yaml": &fstest.MapFile{Data: []byte(`not: valid: yaml: [`)},
			},
			metadata: &project.Metadata{
				Project: project.Project{
					Tag:         "knative-v1.17",
					ImagePrefix: "knative-eventing",
				},
			},
			wantErr: true,
		},
		{
			name: "component repo: returns repo version for main branch",
			fs:   repoAndSOConfig("knative-eventing", "release-next", "1.25", "main", "1.23"),
			metadata: &project.Metadata{
				Project: project.Project{
					Tag:         "knative-nightly",
					ImagePrefix: "knative-eventing",
				},
			},
			wantVersion: "1.25",
		},
		{
			name: "component repo: returns repo version for release-1.38",
			fs:   repoAndSOConfig("knative-serving", "release-v1.21", "1.24", "release-1.38", "1.23"),
			metadata: &project.Metadata{
				Project: project.Project{
					Tag:         "knative-v1.21",
					ImagePrefix: "knative-serving",
				},
			},
			wantVersion: "1.24",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := goVersionFromConfig(tt.fs, tt.metadata)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %q", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result, got nil")
			}
			if *got != tt.wantVersion {
				t.Errorf("got %q, want %q", *got, tt.wantVersion)
			}
		})
	}
}
