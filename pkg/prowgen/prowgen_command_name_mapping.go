package prowgen

import (
	"log"
	"strings"
)

const (
	// maxNameLength is the maximum length for `As` for cluster claim-based tests
	maxNameLength = 42
)

// ToName creates a test name for the given Test following the constraints in openshift/release.
// - name cannot be longer than maxNameLength characters.
func ToName(r Repository, test *Test) string {
	continuousSuffix := "-c"
	maxCommandLength := maxNameLength - len(continuousSuffix)
	if len(test.Command) > maxCommandLength {
		sha := test.HexSha() // guarantees uniqueness
		prefix := test.Command[:maxCommandLength-len(sha)-1]
		// OpenShift CI doesnt' like double dashes, such as `stable-latest-test-kafka--7465737-aws-ocp-412`.
		// So, if the prefix of the command ends with a dash, we remove it.
		prefix = strings.TrimSuffix(prefix, "-")
		newTarget := prefix + "-" + sha
		log.Println(r.RepositoryDirectory(), "command as test name is too long",
			test.Command, "truncating it to", newTarget)
		return newTarget
	}

	return test.Command
}
