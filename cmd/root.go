package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sreootb",
	Short: "SRE: Out of the Box monitoring solution",
	Long: `SRE: Out of the Box (SREootb) is a comprehensive website monitoring 
and alerting solution that provides both server and agent functionality
in a single binary.

Available modes:
  standalone  - Run server + local agent in single process (recommended for simple deployments)
  server      - Run monitoring server only
  agent       - Run monitoring agent only`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check for global version flag when no subcommand is provided
		if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
			fmt.Printf("SREootb %s\n", Version)
			return
		}
		// If no subcommand and no version flag, show help
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./sreootb.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (trace, debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "console", "log format (console, json)")

	// Global version flag
	rootCmd.Flags().BoolP("version", "V", false, "show version information")

	// Bind flags to viper
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name "sreootb" (without extension).
		viper.AddConfigPath(".")
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName("sreootb")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		// Don't log config file usage during version check
		if versionFlag, _ := rootCmd.Flags().GetBool("version"); !versionFlag {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
