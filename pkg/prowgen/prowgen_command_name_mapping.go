package prowgen

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
)

const (
	// maxNameLength is the maximum length for `As` for cluster claim-based tests
	maxNameLength = 42
)

// ToName creates a test name for the given Test following the constraints in openshift/release.
// - name cannot be longer than maxNameLength characters.
func ToName(r Repository, test *Test, openShiftVersion string) string {

	name := test.Command
	if test.OnDemand {
		name = fmt.Sprintf("%s-ondemand", name)
	}

	variant := strings.ReplaceAll(openShiftVersion, ".", "")
	suffix := fmt.Sprintf("-aws-ocp-%s", variant)
	continuousSuffix := "-continuous"

	maxCommandLength := maxNameLength - len(suffix) - len(continuousSuffix)
	if len(name) > maxCommandLength {
		sha := hexSha(name) // guarantees uniqueness
		prefix := name[:maxCommandLength-len(sha)-1]
		if strings.HasSuffix(prefix, "-") {
			// OpenShift CI doesnt' like double dashes, such as `stable-latest-test-kafka--7465737-aws-ocp-412`.
			// So, if the prefix of the command ends with a dash, we remove it.
			prefix = prefix[:len(prefix)-1]
		}
		newTarget := prefix + "-" + sha
		log.Println(r.RepositoryDirectory(), "command as test name is too long", name, "truncating it to", newTarget)
		return fmt.Sprintf("%s%s", newTarget, suffix)
	}

	return fmt.Sprintf("%s%s", name, suffix)
}

func hexSha(name string) string {
	h := sha1.New()
	h.Write([]byte(name))
	return hex.EncodeToString(h.Sum(nil))[:shaLength]
}
