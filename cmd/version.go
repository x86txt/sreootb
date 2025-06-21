package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// Version information - set via ldflags during build
	Version   = "dev"     // Version number
	Commit    = "unknown" // Git commit hash
	Date      = "unknown" // Build date
	GoVersion = runtime.Version()
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display version, build information, and system details for SREootb.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("SREootb %s\n", Version)

		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			fmt.Printf("Version:    %s\n", Version)
			fmt.Printf("Commit:     %s\n", Commit)
			fmt.Printf("Build Date: %s\n", Date)
			fmt.Printf("Go Version: %s\n", GoVersion)
			fmt.Printf("Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolP("verbose", "v", false, "show detailed version information")
}
