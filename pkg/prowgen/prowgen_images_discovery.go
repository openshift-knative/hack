package prowgen

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asottile/dockerfile"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/strings/slices"
)

const (
	srcImage = "src"
)

var (
	ciRegistryRegex = regexp.MustCompile(`registry\.(|svc\.)ci\.openshift\.org/\S+`)

	ubiMinimal8Regex = regexp.MustCompile(`registry\.access\.redhat\.com/ubi8-minimal:latest$`)
	ubiMinimal9Regex = regexp.MustCompile(`registry\.access\.redhat\.com/ubi9-minimal:latest$`)

	imageOverrides = map[*regexp.Regexp]orgRepoTag{
		ubiMinimal8Regex: {Org: "ocp", Repo: "ubi-minimal", Tag: "8"},
		ubiMinimal9Regex: {Org: "ocp", Repo: "ubi-minimal", Tag: "9"},
	}

	defaultDockerfileIncludes = []string{
		"openshift/ci-operator/knative-images.*",
		"openshift/ci-operator/knative-test-images.*",
		"openshift/ci-operator/static-images.*",
		"openshift/ci-operator/.*images?.*",
	}
	defaultDockerfileExcludes = []string{
		"openshift/ci-operator/source-image.*",
		"openshift/ci-operator/build-image.*",
	}
)

type orgRepoTag struct {
	Org  string
	Repo string
	Tag  string
}

func (ort orgRepoTag) String() string {
	return ort.Org + "_" + ort.Repo + "_" + ort.Tag
}

func DiscoverImages(r Repository, skipDockerFiles []string) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		log.Println(r.RepositoryDirectory(), "Discovering images")
		opts, err := discoverImages(r, skipDockerFiles)
		if err != nil {
			return err
		}

		return applyOptions(cfg, opts...)
	}
}

func discoverImages(r Repository, skipDockerFiles []string) ([]ReleaseBuildConfigurationOption, error) {
	dockerfiles, err := discoverDockerfiles(r, skipDockerFiles)
	if err != nil {
		return nil, err
	}
	sort.Strings(dockerfiles)

	log.Println(r.RepositoryDirectory(), "Discovered Dockerfiles", dockerfiles)

	options := make([]ReleaseBuildConfigurationOption, 0, len(dockerfiles))

	for _, dockerfile := range dockerfiles {
		requiredBaseImages, inputImages, err := discoverInputImages(dockerfile)
		if err != nil {
			return nil, err
		}

		options = append(options,
			WithBaseImages(r.BaseImages),
			WithBaseImages(requiredBaseImages),
			WithImage(ProjectDirectoryImageBuildStepConfigurationFuncFromImageInput(r, ImageInput{
				Context:        discoverImageContext(dockerfile),
				DockerfilePath: strings.Join(strings.Split(dockerfile, string(os.PathSeparator))[2:], string(os.PathSeparator)),
				Inputs:         computeInputs(r, inputImages, dockerfile),
			})),
		)
	}

	return options, nil
}

func discoverImageContext(dockerfile string) imageContext {
	context := ProductionContext
	if strings.Contains(dockerfile, "test-images") {
		context = TestContext
	}
	return context
}

func discoverDockerfiles(r Repository, skipDockerFiles []string) ([]string, error) {
	dockerFilesToInclude := defaultDockerfileIncludes
	if len(r.Dockerfiles.Matches) != 0 {
		dockerFilesToInclude = r.Dockerfiles.Matches
	}
	dockerFilesToExclude := defaultDockerfileExcludes
	if len(r.Dockerfiles.Excludes) != 0 {
		dockerFilesToExclude = r.Dockerfiles.Excludes
	}
	filteredDockerFiles := slices.Filter(nil, dockerFilesToInclude, func(s string) bool {
		return !slices.Contains(skipDockerFiles, s)
	})
	includePathRegex := ToRegexp(filteredDockerFiles)
	excludePathRegex := ToRegexp(dockerFilesToExclude)
	dockerfiles := sets.NewString()
	rootDir := r.RepositoryDirectory()
	err := filepath.Walk(rootDir, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(info.Name(), "Dockerfile") {
			return nil
		}
		path = filepath.Join(".", strings.TrimPrefix(path, rootDir))

		include := true
		if len(includePathRegex) > 0 {
			include = false
			for _, r := range includePathRegex {
				if r.MatchString(path) {
					include = true
					break
				}
			}
		}
		if !include {
			return nil
		}
		for _, r := range excludePathRegex {
			if r.MatchString(path) {
				include = false
				break
			}
		}
		if !include {
			return nil
		}
		dockerfiles.Insert(filepath.Join(rootDir, path))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed while discovering container images: %w", err)
	}

	srcImageDockerfile, err := discoverSourceImageDockerfile(r)
	if err != nil {
		return nil, err
	}
	if srcImageDockerfile != "" {
		dockerfiles.Insert(srcImageDockerfile)
	}

	return dockerfiles.List(), nil
}

func discoverSourceImageDockerfile(r Repository) (string, error) {
	srcImageDockerfile := filepath.Join(r.RepositoryDirectory(), "openshift", "ci-operator", "source-image", "Dockerfile")
	if _, err := os.Stat(srcImageDockerfile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return srcImageDockerfile, nil
}

func discoverInputImages(dockerfile string) (map[string]cioperatorapi.ImageStreamTagReference, map[string]cioperatorapi.ImageBuildInputs, error) {
	imagePaths, err := getPullStringsFromDockerfile(dockerfile)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get pull images from dockerfile: %w", err)
	}

	requiredBaseImages := make(map[string]cioperatorapi.ImageStreamTagReference)
	inputImages := make(map[string]cioperatorapi.ImageBuildInputs)

	for _, imagePath := range imagePaths {
		if imagePath == srcImage {
			inputImages[srcImage] = cioperatorapi.ImageBuildInputs{As: []string{srcImage}}
		} else {
			orgRepoTag, err := orgRepoTagFromPullString(imagePath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse string %s as pullspec: %w", imagePath, err)
			}

			for k, override := range imageOverrides {
				if k.FindString(imagePath) != "" {
					orgRepoTag = override
					break
				}
			}

			inputs := inputImages[orgRepoTag.String()]
			inputs.As = sets.NewString(inputs.As...).Insert(imagePath).List() //different registries can resolve to the same orgRepoTag

			if orgRepoTag.Org == "_" {
				// consider image as a base image alias
				inputImages[imagePath] = inputs
			} else {
				requiredBaseImages[orgRepoTag.String()] = cioperatorapi.ImageStreamTagReference{
					Namespace: orgRepoTag.Org,
					Name:      orgRepoTag.Repo,
					Tag:       orgRepoTag.Tag,
				}
				inputImages[orgRepoTag.String()] = inputs
			}
		}
	}

	return requiredBaseImages, inputImages, nil
}

func getPullStringsFromDockerfile(filename string) ([]string, error) {
	cmds, err := dockerfile.ParseFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open Dockerfile %s: %w", filename, err)
	}

	images := make([]string, 0, 1)
	for _, cmd := range cmds {
		if cmd.Cmd == "FROM" {
			if len(cmd.Value) != 1 {
				return nil, fmt.Errorf("expected one value for FROM "+
					"command, got %d: %q", len(cmd.Value), cmd.Value)
			}
			images = append(images, cmd.Value[0])
			continue
		}
		if cmd.Cmd == "COPY" || cmd.Cmd == "ADD" {
			for _, fl := range cmd.Flags {
				if strings.HasPrefix(fl, "--from=") {
					images = append(images, strings.TrimPrefix(fl, "--from="))
				}
			}
		}
	}

	return images, nil
}

func computeInputs(
	r Repository,
	images map[string]cioperatorapi.ImageBuildInputs,
	dockerfile string,
) map[string]cioperatorapi.ImageBuildInputs {
	if isSourceOrBuildDockerfile(dockerfile) {
		return images
	}
	inputs := make(map[string]cioperatorapi.ImageBuildInputs, len(r.SharedInputs)+len(images))
	for k, v := range r.SharedInputs {
		inputs[k] = v
	}
	for k, v := range images {
		inputs[k] = v
	}
	return inputs
}

func isSourceOrBuildDockerfile(dockerfile string) bool {
	return strings.Contains(dockerfile, "source-image") ||
		strings.Contains(dockerfile, "build-image")
}

func orgRepoTagFromPullString(pullString string) (orgRepoTag, error) {
	res := orgRepoTag{Tag: "latest"}
	slashSplit := strings.Split(pullString, "/")
	switch n := len(slashSplit); n {
	case 1:
		res.Org = "_"
		res.Repo = slashSplit[0]
	case 2:
		res.Org = slashSplit[0]
		res.Repo = slashSplit[1]
	case 3:
		res.Org = slashSplit[1]
		res.Repo = slashSplit[2]
	default:
		return res, fmt.Errorf("pull string %q couldn't be parsed, expected to get between one and three elements after slashsplitting, got %d", pullString, n)
	}
	if repoTag := strings.Split(res.Repo, ":"); len(repoTag) == 2 {
		res.Repo = repoTag[0]
		res.Tag = repoTag[1]
	}

	return res, nil
}

func ToRegexp(s []string) []*regexp.Regexp {
	includesRegex := make([]*regexp.Regexp, 0, len(s))
	for _, i := range s {
		r, err := regexp.Compile(i)
		if err != nil {
			log.Fatal("Regex", i, "doesn't compile", err)
		}
		includesRegex = append(includesRegex, r)
	}
	return includesRegex
}
