package prowgen

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/openshift-knative/hack/pkg/util"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/strings/slices"
)

const (
	srcImage = "src"
)

var (
	ciRegistryRegex   = regexp.MustCompile(`registry\.(|svc\.)ci\.openshift\.org/\S+`)
	ubiMinimal8Regex1 = regexp.MustCompile(`registry\.access\.redhat\.com/ubi8-minimal\S*`)
	ubiMinimal8Regex2 = regexp.MustCompile(`registry\.access\.redhat\.com/ubi8/ubi-minimal\S*`)
	ubiMinimal9Regex1 = regexp.MustCompile(`registry\.access\.redhat\.com/ubi9-minimal\S*`)
	ubiMinimal9Regex2 = regexp.MustCompile(`registry\.access\.redhat\.com/ubi9/ubi-minimal\S*`)

	imageOverrides = map[*regexp.Regexp]orgRepoTag{
		ubiMinimal8Regex1: {Org: "ocp", Repo: "ubi-minimal", Tag: "8"},
		ubiMinimal8Regex2: {Org: "ocp", Repo: "ubi-minimal", Tag: "8"},
		ubiMinimal9Regex1: {Org: "ocp", Repo: "ubi-minimal", Tag: "9"},
		ubiMinimal9Regex2: {Org: "ocp", Repo: "ubi-minimal", Tag: "9"},
	}

	defaultDockerfileIncludes = []string{
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
			WithBaseImages(requiredBaseImages),
			WithImage(ProjectDirectoryImageBuildStepConfigurationFuncFromImageInput(r, ImageInput{
				Context:        discoverImageContext(dockerfile),
				DockerfilePath: strings.Join(strings.Split(dockerfile, string(os.PathSeparator))[2:], string(os.PathSeparator)),
				Inputs:         inputImages,
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
	skips := make([]*regexp.Regexp, 0, len(skipDockerFiles))
	for _, dockerfile := range skipDockerFiles {
		skips = append(skips, regexp.MustCompile(dockerfile))
	}
	dockerFilesToExclude := defaultDockerfileExcludes
	if len(r.Dockerfiles.Excludes) != 0 {
		dockerFilesToExclude = r.Dockerfiles.Excludes
	}

	filteredDockerFiles := slices.Filter(nil, dockerFilesToInclude, func(s string) bool {
		return !slices.Contains(skipDockerFiles, s)
	})

	includePathRegex, err := util.ToRegexp(filteredDockerFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filtered dockerfile regexp: %v", err)
	}
	excludePathRegex, err := util.ToRegexp(dockerFilesToExclude)
	if err != nil {
		return nil, fmt.Errorf("failed to parse excludes regexp: %v", err)
	}

	dockerfiles := sets.NewString()
	rootDir := r.RepositoryDirectory()
	err = filepath.Walk(rootDir, func(path string, info fs.FileInfo, err error) error {
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
		for _, s := range skips {
			if s.MatchString(path) {
				return nil
			}
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
			orgRepoTag, err := getOrgRepoTag(imagePath)
			if err != nil {
				return nil, nil, err
			}

			requiredBaseImages[orgRepoTag.String()] = cioperatorapi.ImageStreamTagReference{
				Namespace: orgRepoTag.Org,
				Name:      orgRepoTag.Repo,
				Tag:       orgRepoTag.Tag,
			}

			inputs := inputImages[orgRepoTag.String()]
			inputs.As = sets.NewString(inputs.As...).Insert(imagePath).List() //different registries can resolve to the same orgRepoTag
			inputImages[orgRepoTag.String()] = inputs
		}
	}

	// Also parse args and possibly generate inputs for any pull specs defined in args.
	args, err := getArgsWithPullSpec(dockerfile)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get args from dockerfile: %w", err)
	}
	for arg, imagePath := range args {
		orgRepoTag, err := getOrgRepoTag(imagePath)
		if err != nil {
			return nil, nil, err
		}
		requiredBaseImages[orgRepoTag.String()] = cioperatorapi.ImageStreamTagReference{
			Namespace: orgRepoTag.Org,
			Name:      orgRepoTag.Repo,
			Tag:       orgRepoTag.Tag,
		}
		inputs := inputImages[orgRepoTag.String()]
		// Add the arg as variable formatted as $XYZ. This allows specifying "FROM $XYZ" in Dockerfile
		// and CI operator will still be able to replace the image.
		inputs.As = sets.NewString(inputs.As...).Insert(fmt.Sprintf("$%s", arg)).List()
		inputImages[orgRepoTag.String()] = inputs
	}

	return requiredBaseImages, inputImages, nil
}

func getOrgRepoTag(imagePath string) (*orgRepoTag, error) {
	orgRepoTag, err := orgRepoTagFromPullString(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse string %s as pullspec: %w", imagePath, err)
	}
	for k, override := range imageOverrides {
		if k.FindString(imagePath) != "" {
			orgRepoTag = override
			break
		}
	}
	return &orgRepoTag, nil
}

func getPullStringsFromDockerfile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open Dockerfile %s: %w", filename, err)
	}
	defer file.Close()

	var images []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !(strings.Contains(line, "FROM ") || strings.Contains(line, "--from=")) {
			continue
		}

		if match := matchKnownPullSpec(line); match != "" {
			images = append(images, match)
		}

		if line == "FROM src" {
			images = append(images, srcImage)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read Dockerfile %s: %w", filename, err)
	}

	return images, nil
}

func getArgsWithPullSpec(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open Dockerfile %s: %w", filename, err)
	}
	defer file.Close()

	args := make(map[string]string)
	scanner := bufio.NewScanner(file)
	prefix := "ARG "
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			argVal := strings.TrimPrefix(line, prefix)
			split := strings.Split(argVal, "=")
			if len(split) == 2 {
				if match := matchKnownPullSpec(split[1]); match != "" {
					args[split[0]] = match
				}
			}
		}
	}

	return args, nil
}

func matchKnownPullSpec(line string) string {
	var match string
	match = ciRegistryRegex.FindString(line)
	if match != "" {
		return match
	}
	// Also include any images for which there are overrides.
	for r := range imageOverrides {
		match := r.FindString(line)
		if match != "" {
			return match
		}
	}
	return ""
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
