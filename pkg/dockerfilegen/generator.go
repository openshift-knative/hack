package dockerfilegen

import (
	"bytes"
	"embed"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/openshift-knative/hack/config"
	"github.com/openshift-knative/hack/pkg/soversion"

	"github.com/coreos/go-semver/semver"
	"github.com/openshift-knative/hack/pkg/project"
	"github.com/openshift-knative/hack/pkg/prowgen"
	"github.com/openshift-knative/hack/pkg/rhel"
	"github.com/openshift-knative/hack/pkg/util"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	GenerateDockerfileOption           = "dockerfile"
	GenerateMustGatherDockerfileOption = "must-gather-dockerfile"
	DefaultDockerfileTemplateName      = "default"
	FuncUtilDockerfileTemplateName     = "func-util"

	defaultAppFilename               = "main"
	mustGatherDockerfileTemplateName = "must-gather"
	ocClientArtifactsBaseImage       = "registry.ci.openshift.org/ocp/%s:cli-artifacts"
	// See https://github.com/containerbuildsystem/cachi2/blob/3c562a5410ddd5f1043e7613b240bb5811682f7f/cachi2/core/package_managers/rpm/main.py#L29
	cachi2DefaultRPMsLockFilePath = "rpms.lock.yaml"

	RHEL8 = "rhel-8"
	RHEL9 = "rhel-9"
)

var (
	ErrUnexpected      = fmt.Errorf("unexpected")
	ErrIO              = fmt.Errorf("%w: system IO failed", ErrUnexpected)
	ErrBadConf         = fmt.Errorf("bad configuration")
	ErrUnsupportedRepo = fmt.Errorf("unsupported repo structure")
	ErrBadTemplate     = fmt.Errorf("bad template")
)

// GenerateDockerfiles will generate Dockerfile files
func GenerateDockerfiles(params Params) error {
	if params.RootDir == "" {
		return fmt.Errorf("%w: root-dir cannot be empty", ErrBadConf)
	}
	if !path.IsAbs(params.Output) {
		params.Output = path.Join(params.RootDir, params.Output)
	}
	if !path.IsAbs(params.ProjectFilePath) {
		params.ProjectFilePath = path.Join(params.RootDir, params.ProjectFilePath)
	}

	if err := os.Chdir(params.RootDir); err != nil {
		return fmt.Errorf("%w: Chdir: %w",
			ErrIO, errors.WithStack(err))
	}

	{
		var err error
		if params.RootDir, err = os.Getwd(); err != nil {
			return fmt.Errorf("%w: Getwd: %w",
				ErrIO, errors.WithStack(err))
		}
	}

	includesRegex := util.MustToRegexp(params.Includes)
	excludesRegex := util.MustToRegexp(params.Excludes)

	mainPackagesPaths := sets.New[string]()

	if err := filepath.Walk(params.RootDir, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		path = filepath.Join(".", strings.TrimPrefix(path, params.RootDir))

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
	}); err != nil {
		return errors.WithStack(err)
	}

	for _, p := range mainPackagesPaths.UnsortedList() {
		log.Println("Main package path:", p)
	}

	if params.Generators == GenerateDockerfileOption {
		return generateDockerfile(params, mainPackagesPaths)
	} else if params.Generators == GenerateMustGatherDockerfileOption {
		return generateMustGatherDockerfile(params)
	}

	return fmt.Errorf("%w: unsupported generator choosen: `%s`",
		ErrBadConf, params.Generators)
}

func generateDockerfile(params Params, mainPackagesPaths sets.Set[string]) error {
	goMod, err := getGoMod(params.RootDir)
	if err != nil {
		return err
	}

	metadata, err := project.ReadMetadataFile(params.ProjectFilePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: Failed to read project metadata file: %w",
				ErrBadConf, errors.WithStack(err))
		}
		log.Println("File not found:", params.ProjectFilePath, "(Using defaults)")
		metadata = project.DefaultMetadata()
	}

	var goVersion string
	if goVersion, err = resolveGolangVersion(params, goMod, metadata); err != nil {
		return err
	}

	rhelVersion := RHEL9
	templateFilePattern := "dockerfile-templates/rhel-9/*.tmpl"
	var soVersion string
	projectTag := metadata.Project.Tag
	if projectTag == "main" || metadata.Project.Tag == "" {
		projectTag = "knative-v1.17"
		log.Println("Use Default version substitution for main branch")
	}
	if projectTag != "" && projectTag != "main" {
		// Handle knative-vX.Y tags
		// tag before knative-v1.17
		if strings.HasPrefix(projectTag, "knative-v1.") {
			minorVersion, err := strconv.Atoi(strings.Replace(projectTag, "knative-v1.", "", 1))
			if err != nil {
				log.Printf("Failed to parse minor version from tag %q: %v", projectTag, err)
				minorVersion = 17 // fallback
			}
			if minorVersion < 17 {
				rhelVersion = RHEL8
				templateFilePattern = "dockerfile-templates/*.tmpl"
			}
			semverSoVersion := soversion.FromUpstreamVersion(strings.Replace(projectTag, "knative-v", "", 1))
			soVersion = fmt.Sprintf("%v.%v", semverSoVersion.Major, semverSoVersion.Minor)
		}
	} else {
		if metadata.Project.Version == "main" || metadata.Project.Version == "" {
			metadata.Project.Version = "1.37.0" //fallback
		}
		// version before 1.37+
		if metadata.Project.Version != "" {
			semVer := semver.New(metadata.Project.Version)
			if semVer != nil {
				if semVer.Minor < 37 {
					rhelVersion = RHEL8
					templateFilePattern = "dockerfile-templates/*.tmpl"
				}
			}
			soVersion = fmt.Sprintf("%v.%v", semVer.Major, semVer.Minor)
		}
	}

	builderImage := params.DockerfileImageBuilderFmt
	if builderImage == "" {
		builderImage = builderImageForGoVersion(goVersion, rhelVersion)
	} else {
		// Builder image might be provided without formatting '%s' string as plain value
		if strings.Count(params.DockerfileImageBuilderFmt, "%s") == 1 {
			builderImage = fmt.Sprintf(params.DockerfileImageBuilderFmt, goVersion)
		}
	}

	goPackageToImageMapping := map[string]string{}

	d := map[string]interface{}{
		"builder": builderImage,
	}
	if _, err = saveDockerfile(d, DockerfileBuildImageTemplate, templateFilePattern, params.Output, params.DockerfilesBuildDir); err != nil {
		return err
	}

	if _, err = saveDockerfile(d, DockerfileSourceImageTemplate, templateFilePattern, params.Output, params.DockerfilesSourceDir); err != nil {
		return err
	}

	if len(params.AdditionalPackages) > 0 {
		params.RpmsLockFileEnabled = true
	}

	var additionalInstructions []string
	if slices.Contains(params.AdditionalPackages, "tzdata") {
		// https://access.redhat.com/solutions/5616681
		additionalInstructions = append(additionalInstructions, fmt.Sprintf("RUN microdnf update tzdata -y && microdnf reinstall tzdata -y"))
		idx := -1
		for i, p := range params.AdditionalPackages {
			if strings.TrimSpace(p) == "tzdata" {
				idx = i
				break
			}
		}
		if idx >= 0 {
			params.AdditionalPackages = slices.Delete(params.AdditionalPackages, idx, idx+1)
		}
	}
	if len(params.AdditionalPackages) > 0 {
		additionalInstructions = append(additionalInstructions, fmt.Sprintf("RUN microdnf install %s", strings.Join(params.AdditionalPackages, " ")))
	}

	buildEnvs := make([]string, 0, len(DefaultBuildEnvVars())+len(params.AdditionalBuildEnvVars)+1)
	buildEnvs = append(buildEnvs, DefaultBuildEnvVars()...)
	vendored, err := hasVendorFolder(params.RootDir)
	if err != nil {
		return fmt.Errorf("could not check if project is vendorless: %w", err)
	}
	if !vendored {
		buildEnvs = append(buildEnvs, "GOFLAGS='-mod=mod'")
	}

	if len(params.AdditionalBuildEnvVars) > 0 {
		buildEnvs = append(buildEnvs, params.AdditionalBuildEnvVars...)
	}

	rpmsLockFileWritten := false
	for _, p := range mainPackagesPaths.UnsortedList() {
		appFile := fmt.Sprintf(params.AppFileFmt, appFilename(p))
		projectName := strings.TrimPrefix(metadata.Project.ImagePrefix, "knative-")
		var projectWithSep, projectDashCaseWithSep string
		if projectName != "" {
			projectWithSep = capitalize(projectName) + " "
			projectDashCaseWithSep = projectName + "-"
		}
		// Compute base values
		componentDashcase := dashcase(p)
		projectDashcase := projectDashCaseWithSep

		// Apply overrides
		labelName := adjustComponentName(projectDashcase, componentDashcase, strings.ReplaceAll(rhelVersion, "-", ""))

		elVersion := "el" + strings.TrimPrefix(rhelVersion, "rhel-")
		d := map[string]interface{}{
			"main":                    p,
			"app_file":                appFile,
			"builder":                 builderImage,
			"version":                 metadata.Project.Tag,
			"project":                 projectWithSep,
			"project_dashcase":        projectDashCaseWithSep,
			"component":               capitalize(p),
			"component_dashcase":      dashcase(p),
			"additional_instructions": additionalInstructions,
			"build_env_vars":          buildEnvs,
			"rhelVersion":             strings.ReplaceAll(rhelVersion, "-", ""),
			"short_version":           soVersion,
			"shortRhelVersion":        elVersion,
			"name":                    labelName,
		}

		var dockerfileTemplate embed.FS
		var rpmsLockTemplate *embed.FS
		if params.RpmsLockFileEnabled {
			if rhelVersion == RHEL8 {
				rpmsLockTemplate = &RPMsLockTemplateUbi8
			} else {
				rpmsLockTemplate = &RPMsLockTemplateUbi9
			}
		}
		// var templateFile string
		switch params.TemplateName {
		case DefaultDockerfileTemplateName:
			dockerfileTemplate = DockerfileDefaultTemplate
		case FuncUtilDockerfileTemplateName:
			if rhelVersion == RHEL9 {
				rpmsLockTemplate = &RPMsLockTemplateUbi9
			} else {
				rpmsLockTemplate = &RPMsLockTemplateUbi8
			}
			dockerfileTemplate = DockerfileFuncUtilTemplate
		default:
			return fmt.Errorf("%w: Unknown template name: %s",
				ErrBadConf, params.TemplateName)
		}
		templateFiles := "dockerfile-templates/*.tmpl"
		if rhelVersion == RHEL9 {
			templateFiles = "dockerfile-templates/rhel-9/*.tmpl"
		}
		t, err := template.ParseFS(dockerfileTemplate, templateFiles)
		if err != nil {
			return fmt.Errorf("%w: Parsing failed: %w",
				ErrBadTemplate, errors.WithStack(err))
		}

		bf := new(bytes.Buffer)
		if err := t.Execute(bf, d); err != nil {
			return fmt.Errorf("%w: executing failed: %w",
				ErrBadTemplate, errors.WithStack(err))
		}

		out := filepath.Join(params.Output, params.DockerfilesDir, filepath.Base(p))
		context := prowgen.ProductionContext
		if strings.Contains(p, "test") {
			context = prowgen.TestContext
			out = filepath.Join(params.Output, params.DockerfilesTestDir, filepath.Base(p))
		}

		dockerfilePath, err := saveDockerfile(d, dockerfileTemplate, templateFiles, out, "")
		if err != nil {
			return err
		}

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
			return fmt.Errorf("%w: Failed to derive image name: %w",
				ErrUnsupportedRepo, errors.WithStack(err))
		}
		image := fmt.Sprintf(params.RegistryImageFmt, v.To, metadata.Project.Tag)
		if imageEnv := os.Getenv(strings.ToUpper(strings.ReplaceAll(string(v.To), "-", "_"))); imageEnv != "" {
			image = imageEnv
		}
		if strings.HasPrefix(p, "vendor/") {
			goPackageToImageMapping[strings.Replace(p, "vendor/", "", 1)] = image
		} else {
			goPackageToImageMapping[filepath.Join(goMod.Module.Mod.Path, p)] = image
		}

		if rpmsLockTemplate != nil && !rpmsLockFileWritten {
			if err = writeRPMLockFile(rpmsLockTemplate, params.RootDir); err != nil {
				return err
			}
			rpmsLockFileWritten = true
		}
	}

	if err := getAdditionalImagesFromMatchingRepositories(params.ImagesFromRepositories, metadata, params.ImagesFromRepositoriesURLFmt, goPackageToImageMapping); err != nil {
		return fmt.Errorf("%w: Getting additional images from matching repositories failed: %w",
			ErrUnsupportedRepo, errors.WithStack(err))
	}

	mapping, err := yaml.Marshal(goPackageToImageMapping)
	if err != nil {
		return fmt.Errorf("%w: yaml marshaling failed: %w",
			ErrUnexpected, errors.WithStack(err))
	}
	// Write the mapping file between Go packages to resolved images.
	// For example:
	// github.com/openshift-knative/hack/cmd/prowgen: registry.ci.openshift.org/openshift/knative-prowgen:knative-v1.8
	// github.com/openshift-knative/hack/cmd/testselect: registry.ci.openshift.org/openshift/knative-test-testselect:knative-v1.8
	immapPath := filepath.Join(params.Output, "images.yaml")
	if err := os.WriteFile(immapPath, mapping, fs.ModePerm); err != nil {
		return fmt.Errorf("%w: Write images mapping file failed `%s`: %w",
			ErrIO, immapPath, errors.WithStack(err))
	}
	log.Println("Images mapping file written:", immapPath)
	return nil
}

func resolveGolangVersion(params Params, goMod *modfile.File, metadata *project.Metadata) (string, error) {
	goVersion := goMod.Go.Version

	// The builder images are distinguished by golang major.minor, so we ignore the rest of the goVersion
	if strings.Count(goVersion, ".") > 1 {
		goVersion = strings.Join(strings.Split(goVersion, ".")[0:2], ".")
	}

	log.Println("Golang version (from go.mod):", goVersion)

	branch := strings.Replace(metadata.Project.Tag, "knative-", "release-", 1)
	nvBranch := strings.Replace(branch, "release-v", "release-", 1)
	soVer, err := soversion.SoFromUpstreamVersion(nvBranch)
	reponame := params.GetRepoName(goMod)

	resolver := golangResolver{
		defaultVersion: goVersion,
		params: []golangResolverParams{
			{reponame, branch},
			{reponame, nvBranch},
		},
	}

	if err == nil {
		soRelease := soversion.BranchName(soVer)
		resolver.params = append(resolver.params, golangResolverParams{
			"serverless-operator", soRelease,
		})
	}
	
	return resolver.perform()
}

type golangResolverParams struct {
	reponame, branch string
}

type golangResolver struct {
	defaultVersion string
	params         []golangResolverParams
}

func (r golangResolver) perform() (string, error) {
	goVersion := r.defaultVersion
	for _, check := range r.params {
		configGoVersion, err := resolveGolangVersionForRepo(check.reponame, check.branch)
		if err != nil && !errors.Is(err, ErrCantFindConfig) {
			return "", err
		}
		if configGoVersion != "" {
			goVersion = configGoVersion
			log.Println("Golang version (overridden for", check.reponame, "@", check.branch, "):", goVersion)
			break
		}
	}
	return goVersion, nil
}

var ErrCantFindConfig = errors.New("can't find config file")

func resolveGolangVersionForRepo(reponame string, branchName string) (string, error) {
	cfgYaml, err := config.Configs.ReadFile(fmt.Sprint(reponame, ".yaml"))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrCantFindConfig, err)
	}
	prowcfg, perr := prowgen.UnmarshalConfig(cfgYaml)
	if perr != nil {
		return "", errors.WithStack(perr)
	}

	branch := prowcfg.Config.Branches[branchName]
	if branch.GolangVersion != "" {
		goVersion := branch.GolangVersion
		return goVersion, nil
	}
	return "", nil
}

func hasVendorFolder(dir string) (bool, error) {
	info, err := os.Stat(path.Join(dir, "vendor"))
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func generateMustGatherDockerfile(params Params) error {
	var rpmsLockTemplate *embed.FS
	params.TemplateName = mustGatherDockerfileTemplateName
	metadata, err := project.ReadMetadataFile(params.ProjectFilePath)
	if err != nil {
		return fmt.Errorf("%w: Could not read metadata file: %w",
			ErrBadConf, errors.WithStack(err))
	}
	ocClientArtifactsImage := fmt.Sprintf(ocClientArtifactsBaseImage, metadata.Requirements.OcpVersion.Min)
	if metadata.Project.Version != "" {
		// explicit override OCP min version due to RHEL 9 images since 1.37
		if semver.New(metadata.Project.Version).Minor >= 37 && metadata.Requirements.OcpVersion.Min == "4.14" {
			ocClientArtifactsImage = fmt.Sprintf(ocClientArtifactsBaseImage, "4.16")
		}
	}
	projectName := mustGatherDockerfileTemplateName
	projectDashCaseWithSep := projectName + "-"

	ocBinaryName, err := getOCBinaryName(metadata)
	if err != nil {
		return fmt.Errorf("%w: Could not get oc binary name: %w",
			ErrUnsupportedRepo, errors.WithStack(err))
	}
	rhelVersion := RHEL9
	var soVersion string
	pVersion := metadata.Project.Version
	if pVersion != "" {
		semVer := semver.New(pVersion)
		if semVer != nil {
			if semVer.Minor < 37 {
				rhelVersion = RHEL8
			}
		}
		soVersion = fmt.Sprintf("%v.%v", semVer.Major, semVer.Minor)
	}
	elVersion := "el" + strings.TrimPrefix(rhelVersion, "rhel-")

	// Apply overrides
	d := map[string]interface{}{
		"main":             projectName,
		"oc_cli_artifacts": ocClientArtifactsImage,
		"oc_binary_name":   ocBinaryName,
		"version":          metadata.Project.Version,
		"project":          capitalize(projectName),
		"project_dashcase": projectDashCaseWithSep,
		"rhelVersion":      strings.ReplaceAll(rhelVersion, "-", ""),
		"short_version":    soVersion,
		"shortRhelVersion": elVersion,
		"name":             fmt.Sprintf("serverless-must-gather-%s", strings.ReplaceAll(rhelVersion, "-", "")),
	}
	// Pick proper template FS file and RPM lock file
	templateFile := "dockerfile-templates/rhel-9/*.tmpl" // RHEL9 default
	if rhelVersion == RHEL9 {
		rpmsLockTemplate = &RPMsLockTemplateUbi9
	} else {
		templateFile = "dockerfile-templates/*.tmpl"
		rpmsLockTemplate = &RPMsLockTemplateUbi8
	}

	t, err := template.ParseFS(DockerfileMustGatherTemplate, templateFile)
	if err != nil {
		return fmt.Errorf("%w: Parsing failed: %w",
			ErrBadTemplate, errors.WithStack(err))
	}

	bf := new(bytes.Buffer)
	if err := t.Execute(bf, d); err != nil {
		return fmt.Errorf("%w: executing failed: %w",
			ErrBadTemplate, errors.WithStack(err))
	}

	out := filepath.Join(params.Output, params.DockerfilesDir, filepath.Base(projectName))
	if _, err = saveDockerfile(d, DockerfileMustGatherTemplate, templateFile, out, ""); err != nil {
		return err
	}
	if err = writeRPMLockFile(rpmsLockTemplate, params.RootDir); err != nil {
		return err
	}

	return nil
}

func getOCBinaryName(metadata *project.Metadata) (string, error) {
	// depending on the OCP version, the oc binary has different names in registry.ci.openshift.org/ocp/4.13:cli-artifacts:
	// <4.15 it's simply oc, but for >=4.15 it contains two (one for each rhel version: oc.rhel8 & oc.rhel9)

	ocpVersion := metadata.Requirements.OcpVersion.Min

	parts := strings.SplitN(ocpVersion, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid OCP version: %s", ocpVersion)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("could not convert OCP minor to int (%q): %w", ocpVersion, err)
	}

	if minor <= 14 {
		return "oc", nil
	} else {
		// use rhel suffix for OCP version >= 4.15

		soVersion := semver.New(metadata.Project.Version)
		rhelVersion, err := rhel.ForSOVersion(soVersion)
		if err != nil {
			return "", fmt.Errorf("could not determine rhel version: %v", err)
		}

		return fmt.Sprintf("oc.rhel%s", rhelVersion), nil
	}
}

func appFilename(importpath string) string {
	base := filepath.Base(importpath)

	// If we fail to determine a good name from the importpath then use a
	// safe default.
	if base == "." || base == string(filepath.Separator) {
		return defaultAppFilename
	}

	return base
}

func dashcase(path string) string {
	dir := cmdSubPath(path)
	dir = strings.ReplaceAll(dir, "/", "-")
	dir = strings.ReplaceAll(dir, "_", "-")
	return strings.ToLower(dir)
}

func capitalize(path string) string {
	dir := cmdSubPath(path)
	dir = strings.ReplaceAll(dir, "/", " ")
	dir = strings.ReplaceAll(dir, "_", " ")
	dir = strings.ReplaceAll(dir, "-", " ")
	return strings.Title(strings.ToLower(dir))
}

func cmdSubPath(path string) string {
	if _, dir, found := strings.Cut(path, "cmd/"); found {
		return dir
	}
	return path
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

func saveDockerfile(d map[string]interface{}, imageTemplate embed.FS, templatePattern, output string, dir string) (string, error) {
	bt, err := template.ParseFS(imageTemplate, templatePattern)
	if err != nil {
		return "", fmt.Errorf("%w: Failed creating template: %w",
			ErrBadTemplate, errors.WithStack(err))
	}
	bf := &bytes.Buffer{}
	if err := bt.Execute(bf, d); err != nil {
		return "", fmt.Errorf("%w: Failed to execute template: %w",
			ErrBadTemplate, errors.WithStack(err))
	}

	out := filepath.Join(output, dir)
	if err := os.RemoveAll(out); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: os.RemoveAll(\"%s\"): %w",
			ErrIO, out, errors.WithStack(err))
	}
	if err := os.MkdirAll(out, fs.ModePerm); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("%w: os.MkdirAll(\"%s\"): %w",
			ErrIO, out, errors.WithStack(err))
	}
	dockerfilePath := filepath.Join(out, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, bf.Bytes(), fs.ModePerm); err != nil {
		return "", fmt.Errorf("%w: os.WriteFile(\"%s\"): %w",
			ErrIO, dockerfilePath, errors.WithStack(err))
	}

	log.Println("Dockerfile written:", dockerfilePath)
	return dockerfilePath, nil
}

func getGoMod(rootDir string) (*modfile.File, error) {
	goModFile := filepath.Join(rootDir, "go.mod")
	goModContent, err := os.ReadFile(goModFile)
	if err != nil {
		return nil, fmt.Errorf("%w: Failed to read go mod file %s: %w",
			ErrIO, goModFile, errors.WithStack(err))
	}

	gm, err := modfile.Parse(goModFile, goModContent, func(path, version string) (string, error) {
		return version, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: Failed to parse go mod file %s: %w",
			ErrUnsupportedRepo, goModFile, errors.WithStack(err))
	}
	return gm, nil
}

func writeRPMLockFile(rpmsLockTemplate fs.FS, rootDir string) error {
	// Parse the template file
	t, err := template.ParseFS(rpmsLockTemplate, "*.rpms.lock.yaml")
	if err != nil {
		return fmt.Errorf("%w: Failed to parse RPM template: %w",
			ErrBadTemplate, errors.WithStack(err))
	}

	// Create a buffer to hold the template execution output
	bf := new(bytes.Buffer)
	if err := t.Execute(bf, nil); err != nil {
		return fmt.Errorf("%w: Failed to execute RPM template: %w",
			ErrBadTemplate, errors.WithStack(err))
	}

	// Define the output file path
	outputPath := filepath.Join(rootDir, cachi2DefaultRPMsLockFilePath)

	// Write the generated content to the params.Output file
	if err := os.WriteFile(outputPath, bf.Bytes(), 0644); err != nil {
		return fmt.Errorf("%w: Failed to write RPM lock file: %w",
			ErrIO, errors.WithStack(err))
	}
	return nil
}

func builderImageForGoVersion(goVersion, rhelVersion string) string {
	if rhelVersion == "" {
		rhelVersion = "rhel-8"
	}
	builderImageFmt := "registry.ci.openshift.org/openshift/release:%s-release-golang-%s-openshift-%s"

	switch goVersion {
	case "1.21":
		return fmt.Sprintf(builderImageFmt, rhelVersion, goVersion, "4.16")
	case "1.22":
		return fmt.Sprintf(builderImageFmt, rhelVersion, goVersion, "4.17")
	case "1.23":
		return fmt.Sprintf(builderImageFmt, rhelVersion, goVersion, "4.19")
	case "1.24":
		return fmt.Sprintf(builderImageFmt, rhelVersion, goVersion, "4.20")
	case "1.25":
		fallthrough
	default:
		return fmt.Sprintf(builderImageFmt, rhelVersion, goVersion, "4.21")
	}
}

// adjustComponentName applies manual overrides for certain components.
// It returns corrected project_dashcase and component_dashcase values.
func adjustComponentName(projectDashcase, componentDashcase, rhelVersion string) string {
	combinedName := projectDashcase + componentDashcase
	switch combinedName {
	case "ingress":
		combinedName = "serverless-ingress"
	case "knative-operator":
		combinedName = "serverless-kn-operator"
	case "openshift-knative-operator":
		return "serverless-openshift-kn-" + rhelVersion + "-operator"
	//this case if for image name only need `kn-*` prefix
	case "client-cli-artifacts", "client-kn", "eventing-istio-controller", "eventing-apiserver-receive-adapter", "eventing-controller", "eventing-jobsink", "eventing-migrate", "eventing-mtchannel-broker", "eventing-mtping", "eventing-webhook",
		"eventing-integrations-aws-sns-sink", "eventing-integrations-aws-sqs-sink", "eventing-integrations-aws-sqs-source", "eventing-integrations-log-sink", "eventing-integrations-timer-source", "eventing-integrations-transform-jsonata", "eventing-integrations-aws-ddb-streams-source",
		"backstage-plugins-eventmesh", "serving-activator", "serving-autoscaler-hpa", "serving-autoscaler", "serving-controller", "serving-queue", "serving-webhook":
		combinedName = fmt.Sprintf("kn-%s", combinedName)
	case "kn-plugin-event-kn-event-sender":
		combinedName = "kn-plugin-event-sender"
	case "eventing-in-memory-channel-controller":
		combinedName = "kn-eventing-channel-controller"
	case "eventing-in-memory-channel-dispatcher":
		combinedName = "kn-eventing-channel-dispatcher"
	case "eventing-broker-filter":
		combinedName = "kn-eventing-filter"
	case "eventing-broker-ingress":
		combinedName = "kn-eventing-ingress"
	case "eventing-kafka-broker-dispatcher", "eventing-kafka-broker-kafka-controller", "eventing-kafka-broker-post-install", "eventing-kafka-broker-webhook-kafka", "eventing-kafka-broker-receiver":
		combinedName = strings.TrimPrefix(combinedName, "eventing-kafka-broker")
		combinedName = fmt.Sprintf("kn-ekb%s", combinedName)
	case "serving-migrate":
		combinedName = "kn-serving-storage-version-migration"
	default:
		return combinedName + "-" + rhelVersion
	}
	return fmt.Sprintf("%s-%s", combinedName, rhelVersion)
}
