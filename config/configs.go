package config

import "embed"

//go:embed *.yaml
var Configs embed.FS
