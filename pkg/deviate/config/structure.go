package config

// Config for a deviate to operate.
type Config struct {
	Upstream                   string `valid:"required"      yaml:"upstream"`
	Downstream                 string `valid:"required"      yaml:"downstream"`
	DryRun                     bool   `yaml:"dryRun"`
	GithubWorkflowsRemovalGlob string `valid:"required"      yaml:"githubWorkflowsRemovalGlob"`
	ResyncReleases             `yaml:"resyncReleases"`
	Branches                   `yaml:"branches"`
	Tags                       `yaml:"tags"`
	Messages                   `yaml:"messages"`
	SyncLabels                 []string `valid:"required"      yaml:"syncLabels"`
}

// ResyncReleases holds configuration for resyncing past releases.
type ResyncReleases struct {
	Enabled  bool `yaml:"enabled"`
	NumberOf int  `yaml:"numberOf"`
}

// Tags holds configuration for tags.
type Tags struct {
	Synchronize bool   `yaml:"synchronize"`
	RefSpec     string `valid:"required"   yaml:"refSpec"`
}

// Messages holds messages that are used to commit changes and create PRs.
type Messages struct {
	TriggerCI      string `valid:"required" yaml:"triggerCi"`
	TriggerCIBody  string `valid:"required" yaml:"triggerCiBody"`
	ApplyForkFiles string `valid:"required" yaml:"applyForkFiles"`
}

// Branches holds configuration for branches.
type Branches struct {
	Main             string `valid:"required"        yaml:"main"`
	ReleaseNext      string `valid:"required"        yaml:"releaseNext"`
	SynchCI          string `valid:"required"        yaml:"synchCi"`
	ReleaseTemplates `yaml:"releaseTemplates"`
	Searches         `yaml:"searches"`
}

// ReleaseTemplates contains templates for release names.
type ReleaseTemplates struct {
	Upstream   string `valid:"required" yaml:"upstream"`
	Downstream string `valid:"required" yaml:"downstream"`
}

// Searches contains regular expressions used to search for branches.
type Searches struct {
	UpstreamReleases   string `valid:"required" yaml:"upstreamReleases"`
	DownstreamReleases string `valid:"required" yaml:"downstreamReleases"`
}
