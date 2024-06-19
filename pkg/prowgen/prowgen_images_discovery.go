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
	"github.com/distribution/reference"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/strings/slices"
)

const (
	srcImage = "src"
)

var (
	ciRegistryRegex = regexp.MustCompile(`registry\.(|svc\.)ci\.openshift\.org`)

	ubiMinimal8Regex = regexp.MustCompile(`registry\.access\.redhat\.com/ubi8-minimal:latest$`)
	ubiMinimal9Regex = regexp.MustCompile(`registry\.access\.redhat\.com/ubi9-minimal:latest$`)

	imageOverrides = map[*regexp.Regexp]ociReference{
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

type ociReference struct {
	Domain string
	Org    string
	Repo   string
	Tag    string
	Digest string
}

func (ort ociReference) String() string {
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
			ref, err := parseOciReference(imagePath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse string %s as pullspec: %w", imagePath, err)
			}

			for k, override := range imageOverrides {
				if k.FindString(imagePath) != "" {
					ref = override
					break
				}
			}

			inputs := inputImages[ref.String()]
			inputs.As = sets.NewString(inputs.As...).Insert(imagePath).List() //different registries can resolve to the same ociReference

			if ref.Org == "" && ref.Domain == "" {
				// consider image as a base image alias
				inputImages[imagePath] = inputs
				continue
			}
			if !ciRegistryRegex.MatchString(ref.Domain) {
				// don't include images from other registries then registry.ci.openshift.org
				continue
			}

			requiredBaseImages[ref.String()] = cioperatorapi.ImageStreamTagReference{
				Namespace: ref.Org,
				Name:      ref.Repo,
				Tag:       ref.Tag,
			}
			inputImages[ref.String()] = inputs
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
	aliases := make(map[string]string)
	for _, cmd := range cmds {
		if cmd.Cmd == "FROM" {
			if len(cmd.Value) == 1 {
				images = append(images, cmd.Value[0])
				continue
			}
			if len(cmd.Value) == 3 && strings.ToLower(cmd.Value[1]) == "as" {
				aliases[cmd.Value[2]] = cmd.Value[0]
				images = append(images, cmd.Value[0])
				continue
			}
			return nil, fmt.Errorf("invalid FROM stanza: %q", cmd.Value)
		}
		if cmd.Cmd == "COPY" || cmd.Cmd == "ADD" {
			for _, fl := range cmd.Flags {
				if strings.HasPrefix(fl, "--from=") {
					imageOrAlias := strings.TrimPrefix(fl, "--from=")
					if _, ok := aliases[imageOrAlias]; !ok {
						// not an alias, consider it as an image
						images = append(images, imageOrAlias)
					}
				}
			}
		}
	}

	return slices.Filter(make([]string, 0, len(images)), images, func(s string) bool {
		return s != "" && s != "scratch"
	}), nil
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

func parseOciReference(pullString string) (ociReference, error) {
	ref, err := reference.Parse(pullString)
	if err != nil {
		return ociReference{}, fmt.Errorf("failed to parse %q as a reference: %w", pullString, err)
	}
	res := ociReference{Tag: "latest"}
	if named, ok := ref.(reference.Named); ok {
		res.Domain = reference.Domain(named)
		path := strings.SplitN(reference.Path(named), "/", 2)
		if len(path) == 1 {
			res.Repo = path[0]
		} else {
			res.Org = path[0]
			res.Repo = path[1]
		}
		if tagged, ok := ref.(reference.Tagged); ok {
			res.Tag = tagged.Tag()
		}
		if digested, ok := ref.(reference.Digested); ok {
			res.Digest = digested.Digest().String()
		}
		return res, nil
	}

	return res, fmt.Errorf("pull string %q couldn't be parsed as OCI image", pullString)
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
