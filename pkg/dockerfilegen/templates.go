package dockerfilegen

import "embed"

//go:embed dockerfile-templates/Default.dockerfile.tmpl
var DockerfileDefaultTemplate embed.FS

//go:embed dockerfile-templates/FuncUtil.dockerfile.tmpl
var DockerfileFuncUtilTemplate embed.FS

//go:embed dockerfile-templates/BuildImage.dockerfile.tmpl
var DockerfileBuildImageTemplate embed.FS

//go:embed dockerfile-templates/SourceImage.dockerfile.tmpl
var DockerfileSourceImageTemplate embed.FS

//go:embed dockerfile-templates/MustGather.dockerfile.tmpl
var DockerfileMustGatherTemplate embed.FS

//go:embed ubi8.rpms.lock.yaml
var RPMsLockTemplateUbi8 embed.FS

//go:embed ubi9.rpms.lock.yaml
var RPMsLockTemplateUbi9 embed.FS
