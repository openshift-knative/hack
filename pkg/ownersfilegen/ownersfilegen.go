package ownersfilegen

import (
	"bytes"
	"cmp"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

//go:embed owners.tmpl
var OwnersTemplate embed.FS

func WriteOwnersFile(repoDir string, reviewers, approvers []string) error {
	buf := &bytes.Buffer{}

	ownersTemplate, err := template.New("owners.tmpl").ParseFS(OwnersTemplate, "*.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse OWNERS template: %w", err)
	}

	compareIgnoreCase := func(a, b string) int {
		return cmp.Compare(strings.ToLower(a), strings.ToLower(b))
	}
	sortedReviewers := make([]string, len(reviewers))
	sortedApprovers := make([]string, len(approvers))
	copy(sortedReviewers, reviewers)
	copy(sortedApprovers, approvers)
	slices.SortFunc(sortedReviewers, compareIgnoreCase)
	slices.SortFunc(sortedApprovers, compareIgnoreCase)

	d := map[string]interface{}{
		"reviewers": sortedReviewers,
		"approvers": sortedApprovers,
	}

	if err := ownersTemplate.Execute(buf, d); err != nil {
		return fmt.Errorf("failed to execute OWNERS file template: %w", err)
	}

	ownersFilePath := filepath.Join(repoDir, "OWNERS")
	if err := os.WriteFile(ownersFilePath, buf.Bytes(), fs.ModePerm); err != nil {
		return fmt.Errorf("failed to write OWNERS file %q: %w", ownersFilePath, err)
	}

	return nil
}
