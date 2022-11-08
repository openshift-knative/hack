package prowgen

import (
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

	variant := strings.ReplaceAll(openShiftVersion, ".", "")
	suffix := fmt.Sprintf("-aws-ocp-%s", variant)
	continuousSuffix := "-continuous"

	maxCommandLength := maxNameLength - len(suffix) - len(continuousSuffix)
	if len(test.Command) > maxCommandLength {
		sha := test.HexSha() // guarantees uniqueness
		newTarget := test.Command[:maxCommandLength-len(sha)-1] + "-" + sha
		log.Println(r.RepositoryDirectory(), "command as test name is too long", test.Command, "truncating it to", newTarget)
		return fmt.Sprintf("%s%s", newTarget, suffix)
	}

	return fmt.Sprintf("%s%s", test.Command, suffix)
}
