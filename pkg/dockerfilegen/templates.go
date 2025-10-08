package dockerfilegen

import "embed"

//go:embed dockerfile-templates/Default.dockerfile.tmpl dockerfile-templates/rhel-9/*
var DockerfileDefaultTemplate embed.FS

//go:embed dockerfile-templates/FuncUtil.dockerfile.tmpl dockerfile-templates/rhel-9/*
var DockerfileFuncUtilTemplate embed.FS

//go:embed dockerfile-templates/BuildImage.dockerfile.tmpl dockerfile-templates/rhel-9/*
var DockerfileBuildImageTemplate embed.FS

//go:embed dockerfile-templates/SourceImage.dockerfile.tmpl dockerfile-templates/rhel-9/*
var DockerfileSourceImageTemplate embed.FS

//go:embed dockerfile-templates/MustGather.dockerfile.tmpl dockerfile-templates/rhel-9/*
var DockerfileMustGatherTemplate embed.FS

//go:embed ubi8.rpms.lock.yaml
var RPMsLockTemplateUbi8 embed.FS

//go:embed ubi9.rpms.lock.yaml
var RPMsLockTemplateUbi9 embed.FS
