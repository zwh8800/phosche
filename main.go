package main

import (
	"flag"

	"github.com/zwh8800/phosche/internal/app"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	app.Run(webDist, *configPath)
}
