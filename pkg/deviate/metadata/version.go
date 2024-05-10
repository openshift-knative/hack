package metadata

import "runtime/debug"

// Version holds application version information.
var Version = runtimeVersion() //nolint:gochecknoglobals

// VersionPath return a path to the version variable.
func VersionPath() string {
	return importPath("Version")
}

func runtimeVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "v0.0.0"
	}
	return bi.Main.Version
}
