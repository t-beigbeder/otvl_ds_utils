package main

import (
	"cabri"
	"flag"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var debug = true

func usage() {
	log.Fatalf("Incorrect flags please read the documentation")
}

func main() {
	var fDebug = flag.Bool("debug", false, "Displays debug messages and run gin in debug mode")
	var addr = flag.String("addr", "", "The host:port to bind the http server")
	var configName = flag.String("config", "", "The configuration name: S3Read or FSWrite")
	var rootUrl = flag.String("root-url", "", "Root for the URL")
	var rootDir = flag.String("root-dir", "", "Root directory if filesystem")
	flag.Parse()
	if *addr == "" {
		log.Fatalf("Empty addr, please read the documentation")
	}
	if *rootUrl == "" {
		log.Fatalf("Empty root-url, please read the documentation")
	}
	if *configName != "S3Read" && *configName != "FSWrite" {
		log.Fatalf("Incorrect config flag, please read the documentation")
	}
	if *rootDir == "" && *configName == "FSWrite" {
		log.Fatalf("Empty root-dir flag for filesystem, please read the documentation")
	}

	debug = *fDebug
	if debug {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		gin.SetMode(gin.ReleaseMode)
		logrus.SetLevel(logrus.InfoLevel)
	}
	logrus.Info("main: started")
	logrus.Debug("main: see if we are in debug mode")
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	cabri.Run(engine, *addr, *configName, *rootUrl, *rootDir)
	return
}
