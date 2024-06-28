package config

import (
	"github.com/openshift-knative/hack/pkg/deviate/config/git"
	"github.com/openshift-knative/hack/pkg/deviate/log"
	"github.com/openshift-knative/hack/pkg/deviate/log/color"
)

func (c *Config) loadFromGit(
	log log.Logger,
	informer git.RemoteURLInformer,
) error {
	warn := warnFn(log)
	if c.Upstream == "" {
		c.Upstream = remoteURL(informer, "upstream")
		warn("Using upstream remote URL as upstream remote:", c.Upstream)
	}
	if c.Downstream == "" {
		c.Downstream = remoteURL(informer, "downstream")
		if c.Downstream == "" {
			origin := remoteURL(informer, "origin")
			if origin != "" {
				warn("Using origin remote URL as downstream remote:", origin)
				c.Downstream = origin
			}
		} else {
			warn("Using downstream remote URL as downstream remote:", c.Downstream)
		}
	}
	return nil
}

func warnFn(logger log.Logger) func(...interface{}) {
	w := color.Yellow("WARNING:")
	return func(v ...interface{}) {
		v = append([]interface{}{w}, v...)
		logger.Println(v...)
	}
}

func remoteURL(informer git.RemoteURLInformer, remoteName string) string {
	url, _ := informer.Remote(remoteName)
	return url
}
