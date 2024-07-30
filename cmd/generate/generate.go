package main

import (
	"embed"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"text/template"

	"github.com/spf13/pflag"
	"go.uber.org/zap/buffer"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift-knative/hack/pkg/project"
	"github.com/openshift-knative/hack/pkg/prowgen"
)

const (
	GenerateDockerfileOption = "dockerfile"
)

//go:embed Dockerfile.template
var DockerfileTemplate embed.FS

//go:embed BuildImageDockerfile.template
var DockerfileBuildImageTemplate embed.FS

//go:embed SourceImageDockerfile.template
var DockerfileSourceImageTemplate embed.FS

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	var (
		rootDir                      string
		includes                     []string
		excludes                     []string
		generators                   string
		output                       string
		dockerfilesDir               string
		dockerfilesTestDir           string
		dockerfilesBuildDir          string
		dockerfilesSourceDir         string
		projectFilePath              string
		dockerfileImageBuilderFmt    string
		registryImageFmt             string
		imagesFromRepositories       []string
		imagesFromRepositoriesURLFmt string
	)

	defaultIncludes := []string{
		"test/test_images.*",
		"cmd.*",
	}
	defaultExcludes := []string{
		".*k8s\\.io.*",
		".*knative.dev/pkg/codegen.*",
	}

	pflag.StringVar(&rootDir, "root-dir", wd, "Root directory to start scanning, default to current working directory")
	pflag.StringArrayVar(&includes, "includes", defaultIncludes, "File or directory regex to include")
	pflag.StringArrayVar(&excludes, "excludes", defaultExcludes, "File or directory regex to exclude")
	pflag.StringVar(&generators, "generators", "", "Generate something supported: [dockerfile]")
	pflag.StringVar(&dockerfilesDir, "dockerfile-dir", "ci-operator/knative-images", "Dockerfiles output directory for project images relative to output flag")
	pflag.StringVar(&dockerfilesBuildDir, "dockerfile-build-dir", "ci-operator/build-image", "Dockerfiles output directory for build image relative to output flag")
	pflag.StringVar(&dockerfilesSourceDir, "dockerfile-source-dir", "ci-operator/source-image", "Dockerfiles output directory for source image relative to output flag")
	pflag.StringVar(&dockerfilesTestDir, "dockerfile-test-dir", "ci-operator/knative-test-images", "Dockerfiles output directory for test images relative to output flag")
	pflag.StringVar(&output, "output", filepath.Join(wd, "openshift"), "Output directory")
	pflag.StringVar(&projectFilePath, "project-file", filepath.Join(wd, "openshift", "project.yaml"), "Project metadata file path")
	pflag.StringVar(&dockerfileImageBuilderFmt, "dockerfile-image-builder-fmt", "registry.ci.openshift.org/openshift/release:rhel-8-release-golang-%s-openshift-4.17", "Dockerfile image builder format")
	pflag.StringVar(&registryImageFmt, "registry-image-fmt", "registry.ci.openshift.org/openshift/%s:%s", "Container registry image format")
	pflag.StringArrayVar(&imagesFromRepositories, "images-from", nil, "Additional image references to be pulled from other midstream repositories matching the tag in project.yaml")
	pflag.StringVar(&imagesFromRepositoriesURLFmt, "images-from-url-format", "https://raw.githubusercontent.com/openshift-knative/%s/%s/openshift/images.yaml", "Additional images to be pulled from other midstream repositories matching the tag in project.yaml")
	pflag.Parse()

	if rootDir == "" {
		log.Fatal("root-dir cannot be empty")
	}

	if err := os.Chdir(rootDir); err != nil {
		log.Fatal("Chdir", err, string(debug.Stack()))
	}

	rootDir, err = os.Getwd()
	if err != nil {
		log.Fatal("Getwd", err, string(debug.Stack()))
	}

	includesRegex := prowgen.ToRegexp(includes)
	excludesRegex := prowgen.ToRegexp(excludes)

	mainPackagesPaths := sets.NewString()

	err = filepath.Walk(rootDir, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		path = filepath.Join(".", strings.TrimPrefix(path, rootDir))

		include := true
		if len(includesRegex) > 0 {
			include = false
			for _, r := range includesRegex {
				if r.MatchString(path) {
					include = true
					break
				}
			}
		}
		for _, r := range excludesRegex {
			if r.MatchString(path) {
				include = false
				break
			}
		}

		if !include {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("ReadFile %s failed: %w", path, err)
		}
		ast, err := parser.ParseFile(token.NewFileSet(), path, content, parser.PackageClauseOnly)
		if err != nil {
			return fmt.Errorf("ParseFile failed: %w", err)
		}

		if ast.Name.Name != "main" {
			return nil
		}

		mainPackagesPaths.Insert(filepath.Dir(path))
		return nil
	})
	if err != nil {
		log.Fatal(err, "\n", string(debug.Stack()))
	}

	for _, p := range mainPackagesPaths.List() {
		log.Println("Main package path", p)
	}

	if generators == GenerateDockerfileOption {
		goMod := getGoMod(rootDir)
		goVersion := goMod.Go.Version

		builderImage := fmt.Sprintf(dockerfileImageBuilderFmt, goVersion)

		goPackageToImageMapping := map[string]string{}

		metadata, err := project.ReadMetadataFile(projectFilePath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Fatal("Failed to read project metadata file: ", err)
			}
			log.Println("File ", projectFilePath, " not found")
			metadata = nil
		}

		d := map[string]interface{}{
			"builder": builderImage,
		}
		saveDockerfile(d, DockerfileBuildImageTemplate, output, dockerfilesBuildDir)
		saveDockerfile(d, DockerfileSourceImageTemplate, output, dockerfilesSourceDir)

		for _, p := range mainPackagesPaths.List() {
			d := map[string]interface{}{
				"main":    p,
				"builder": builderImage,
			}

			t, err := template.ParseFS(DockerfileTemplate, "*.template")
			if err != nil {
				log.Fatal("Failed creating template ", err)
			}

			bf := &buffer.Buffer{}
			if err := t.Execute(bf, d); err != nil {
				log.Fatal("Failed to execute template", err)
			}

			out := filepath.Join(output, dockerfilesDir, filepath.Base(p))
			context := prowgen.ProductionContext
			if strings.Contains(p, "test") {
				context = prowgen.TestContext
				out = filepath.Join(output, dockerfilesTestDir, filepath.Base(p))
			}

			dockerfilePath := saveDockerfile(d, DockerfileTemplate, out, "")

			if metadata != nil {
				v, err := prowgen.ProjectDirectoryImageBuildStepConfigurationFuncFromImageInput(
					prowgen.Repository{
						ImagePrefix: metadata.Project.ImagePrefix,
					},
					prowgen.ImageInput{
						Context:        context,
						DockerfilePath: dockerfilePath,
					},
				)()
				if err != nil {
					log.Fatal("Failed to derive image name ", err)
				}
				image := fmt.Sprintf(registryImageFmt, v.To, metadata.Project.Tag)
				if imageEnv := os.Getenv(strings.ToUpper(strings.ReplaceAll(string(v.To), "-", "_"))); imageEnv != "" {
					image = imageEnv
				}
				if strings.HasPrefix(p, "vendor/") {
					goPackageToImageMapping[strings.Replace(p, "vendor/", "", 1)] = image
				} else {
					goPackageToImageMapping[filepath.Join(goMod.Module.Mod.Path, p)] = image
				}
			}
		}

		if err := getAdditionalImagesFromMatchingRepositories(imagesFromRepositories, metadata, imagesFromRepositoriesURLFmt, goPackageToImageMapping); err != nil {
			log.Fatal(err)
		}

		mapping, err := yaml.Marshal(goPackageToImageMapping)
		if err != nil {
			log.Fatal(err)
		}
		// Write the mapping file between Go packages to resolved images.
		// For example:
		// github.com/openshift-knative/hack/cmd/prowgen: registry.ci.openshift.org/openshift/knative-prowgen:knative-v1.8
		// github.com/openshift-knative/hack/cmd/testselect: registry.ci.openshift.org/openshift/knative-test-testselect:knative-v1.8
		if err := os.WriteFile(filepath.Join(output, "images.yaml"), mapping, fs.ModePerm); err != nil {
			log.Fatal("Write images mapping file ", err)
		}
	}
}

func getAdditionalImagesFromMatchingRepositories(repositories []string, metadata *project.Metadata, urlFmt string, mapping map[string]string) error {
	branch := strings.Replace(metadata.Project.Tag, "knative", "release", 1)
	branch = strings.Replace(branch, "nightly", "next", 1)
	for _, r := range repositories {
		images, err := downloadImagesFrom(r, branch, urlFmt)
		if err != nil {
			return err
		}

		for k, v := range images {
			// Only add images that are not present
			if _, ok := mapping[k]; !ok {
				log.Println("Additional image from", r, k, v)
				mapping[k] = v
			}
		}
	}

	return nil
}

func downloadImagesFrom(r string, branch string, urlFmt string) (map[string]string, error) {
	url := fmt.Sprintf(urlFmt, r, branch)
	response, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get images for repository %s from %s: %w", r, url, err)
	}
	defer response.Body.Close()

	if response.StatusCode > 400 {
		return nil, fmt.Errorf("failed to get images for repository %s from %s: status code %d", r, url, response.StatusCode)
	}

	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	images := make(map[string]string, 8)
	if err := yaml.Unmarshal(content, images); err != nil {
		return nil, fmt.Errorf("failed to get images for repository %s from %s: %w", r, url, err)
	}
	return images, nil
}

func saveDockerfile(d map[string]interface{}, imageTemplate embed.FS, output string, dir string) string {
	bt, err := template.ParseFS(imageTemplate, "*.template")
	if err != nil {
		log.Fatal("Failed creating template ", err)
	}
	bf := &buffer.Buffer{}
	if err := bt.Execute(bf, d); err != nil {
		log.Fatal("Failed to execute template", err)
	}

	out := filepath.Join(output, dir)
	if err := os.RemoveAll(out); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	}
	if err := os.MkdirAll(out, fs.ModePerm); err != nil && !errors.Is(err, fs.ErrExist) {
		log.Fatal(err)
	}
	dockerfilePath := filepath.Join(out, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, bf.Bytes(), fs.ModePerm); err != nil {
		log.Fatal("Failed writing file", err)
	}

	return dockerfilePath
}

func getGoMod(rootDir string) *modfile.File {
	goModFile := filepath.Join(rootDir, "go.mod")
	goModContent, err := os.ReadFile(goModFile)
	if err != nil {
		log.Fatal("Failed to read go mod file ", goModFile, "error: ", err)
	}

	gm, err := modfile.Parse(goModFile, goModContent, func(path, version string) (string, error) {
		return version, nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return gm
}
