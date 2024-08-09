package deviate

import (
	"hash/crc32"

	"github.com/spf13/cobra"
)

// Option is an option to reconfigure the root command.
type Option func(*cobra.Command)

// Main runs main program.
func Main(opts ...Option) int {
	cmd := root(opts...)
	return hash(cmd.Execute())
}

func hash(err error) int {
	if err == nil {
		return 0
	}
	return int(crc32.ChecksumIEEE([]byte(err.Error())))%254 + 1
}
