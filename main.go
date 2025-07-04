package main

import (
	"embed"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/x86txt/sreootb/cmd"
)

// Embed the Next.js build output - static export mode
//
//go:embed all:web/_next/static
var staticFS embed.FS

//go:embed all:web
var appFS embed.FS

func main() {
	// Initialize logging early
	initLogging()

	// Set the embedded filesystems for the server to use
	cmd.SetWebFS(staticFS, appFS)

	// Execute the root command
	cmd.Execute()
}

func initLogging() {
	// Set default log level
	logLevel := viper.GetString("log.level")
	if logLevel == "" {
		logLevel = "info"
	}

	// Parse log level
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Set log format
	logFormat := viper.GetString("log.format")
	if logFormat == "json" {
		// JSON format for production
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		// Console format for development
		log.Logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Logger()
	}

	// Add caller information in debug mode
	if level <= zerolog.DebugLevel {
		log.Logger = log.Logger.With().Caller().Logger()
	}
}
