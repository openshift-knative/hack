package config

// newDefaults creates a new default configuration.
func newDefaults() Config {
	const (
		releaseTemplate = "release-{{ .Major }}.{{ .Minor }}"
		releaseSearch   = `^release-(\d+)\.(\d+)$`
	)
	return Config{
		GithubWorkflowsRemovalGlob: "knative-*.y?ml",
		Branches: Branches{
			Main:        "main",
			ReleaseNext: "release-next",
			SynchCI:     "ci/",
			ReleaseTemplates: ReleaseTemplates{
				Upstream:   releaseTemplate,
				Downstream: releaseTemplate,
			},
			Searches: Searches{
				UpstreamReleases:   releaseSearch,
				DownstreamReleases: releaseSearch,
			},
		},
		Tags: Tags{
			RefSpec: "v*",
		},
		ResyncReleases: ResyncReleases{
			NumberOf: 6, //nolint:gomnd
		},
		Messages: Messages{
			TriggerCI: ":robot: Synchronize branch `%s` to " +
				"`upstream/%s`",
			TriggerCIBody: "This automated PR is to make sure the " +
				"forked project's `%s` branch (forked upstream's `%s` branch) passes" +
				" a CI.",
			ApplyForkFiles: ":open_file_folder: Apply fork specific files",
		},
		SyncLabels: []string{"kind/sync-fork-to-upstream"},
	}
}
