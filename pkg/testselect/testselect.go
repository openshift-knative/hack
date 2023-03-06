// prowgen generates openshift/release configurations based on the OpenShift serverless
// teams conventions.
//
// For example, it extracts image builds Dockerfile from the common
// directory `openshift/ci-operator/**/Dockerfile.
//
// To onboard a new repository, update the configuration in config/repositories.yaml
// and run the program, or alternatively, you can provide your own configuration file
// using the -config <path> argument.

package testselect

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/openshift-knative/hack/pkg/prowgen"
	"gopkg.in/yaml.v2"
	"k8s.io/test-infra/prow/clonerefs"
)

const (
	all = "All"
)

// TestSuites holds mapping between file path regular expressions and
// test suites that cover the paths.
type TestSuites struct {
	List []TestSuite `json:"testsuites" yaml:"testsuites"`
}

type TestSuite struct {
	Name 		 string   `json:"name" yaml:"name"`
	RunIfChanged []string `json:"run_if_changed" yaml:"run_if_changed"`
	Tests 		 []Test   `json:"tests" yaml:"tests"`
}

type Test struct {
	Name 	 string `json:"name" yaml:"name"`
	Upstream bool   `json:"upstream" yaml:"upstream"`
}

func Main() {
	ctx := context.Background()

	ts := flag.String("testsuites", "testsuites.yaml", "Specify yaml file with path-to-testsuite mapping")
	// Clonerefs options as defined in https://github.com/kubernetes/test-infra/blob/master/prow/clonerefs/options.go
	refs := flag.String("clonerefs", "clonerefs.json", "Specify json file with clonerefs")
	outFile := flag.String("output", "tests.txt", "Specify name of output file")
	flag.Parse()

	log.Println(*ts, *refs, *outFile)

	inRefs, err := os.ReadFile(*refs)
	if err != nil {
		log.Fatalln(err)
	}

	cloneRefs := new(clonerefs.Options)
	if err := json.Unmarshal(inRefs, cloneRefs); err != nil {
		log.Fatalln("Unmarshal clone refs options", err)
	}

	inTs, err := os.ReadFile(*ts)
	if err != nil {
		log.Fatalln(err)
	}

	testSuites := new(TestSuites)
	if err := yaml.UnmarshalStrict(inTs, testSuites); err != nil {
		log.Fatalln("Unmarshal test suite mappings", err)
	}

	//fmt.Printf("%+v", testSuites)

	//log.Printf("Clonerefs:\n %+v\n TestSuites:\n%+v \n", cloneRefs, testSuites)

	if len(cloneRefs.GitRefs) == 0 || len(cloneRefs.GitRefs[0].Pulls) == 0 {
		log.Fatal("Clone refs do not include required SHAs")
	}
	// Fetch base SHA
	prowgen.GitFetch(ctx, cloneRefs.GitRefs[0].RepoLink, cloneRefs.GitRefs[0].BaseSHA)
	// Fetch SHA of pull request commit
	prowgen.GitFetch(ctx, cloneRefs.GitRefs[0].RepoLink, cloneRefs.GitRefs[0].Pulls[0].SHA)
	paths, err := prowgen.GitDiffNameOnly(ctx, cloneRefs.GitRefs[0].BaseSHA, cloneRefs.GitRefs[0].Pulls[0].SHA)
	if err != nil {
		log.Fatalln("Error reading diff", err)
	}
	// "hack/generate/csv.sh", "docs/mesh.md", "hack/lib/serverless.bash"
	paths = []string{ "knative-operator/pkg/webhook/knativeeventing/webhook_mutating.go",
		"openshift-knative-operator/cmd/operator/kodata/monitoring/rbac-proxy.yaml", }
	tests, err := filterTests(*testSuites, paths)
	if err != nil {
		log.Fatal(err)
	}

	var sb strings.Builder
	for _, tst := range tests {
		sb.WriteString(tst + "\n")
	}
	if err := os.WriteFile(*outFile, []byte(sb.String()), os.ModePerm); err != nil {
		log.Fatal(err)
	}
}

func filterTests(testSuites TestSuites, paths []string) ([]string, error) {
	testsToRun := make(map[string]bool)
	for _, path := range paths {
		matchAny := false
		for _, suite := range testSuites.List {
			for _, pathRegex := range suite.RunIfChanged {
				matched, err := regexp.MatchString(pathRegex, path)
				if err != nil {
					return nil, err
				}
				if matched {
					matchAny = true
					for _, test := range suite.Tests {
						testsToRun[test.Name] = true
					}
				}
			}
		}
		// If the path doesn't match any path expressions then it is unknown
		// path and all test suites should be run.
		if !matchAny {
			return []string{ all }, nil
		}
	}

	// If no tests were chosen at this point then the changes don't require any tests.
	// If a "reduced" but non-empty test suite is generated we also want to add tests
	// that don't have any path expression (run_if_changed) and thus should always be added.
	if len(testsToRun) != 0 {
		for _, suite := range testSuites.List {
			if len(suite.RunIfChanged) == 0 {
				for _, test := range suite.Tests {
					testsToRun[test.Name] = true
				}
			}
		}
	}

	return sortedKeys(testsToRun), nil
}

func sortedKeys(stringMap map[string]bool) []string {
	keys := make([]string, 0, len(stringMap))
	for k := range stringMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
