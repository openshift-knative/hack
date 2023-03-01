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
	"fmt"
	"log"
	"os"

	"github.com/openshift-knative/hack/pkg/prowgen"
	"gopkg.in/yaml.v2"
	"k8s.io/test-infra/prow/clonerefs"
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
	flag.Parse()

	log.Println(*ts, *refs)

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
	fmt.Printf("Files changed: %d", len(paths))
}
