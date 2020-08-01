package cabri

import (
	"fmt"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var ActiveServerConfig ServerConfig
var ActiveRootDir string

func Run(engine *gin.Engine, addr string, configName string, rscRoot string, rootDir string) {
	logrus.Debugf("Run configName %s rscRoot %s", configName, rscRoot)
	var ok = true
	if ActiveServerConfig, ok = ServerConfigMap[configName]; !ok {
		log.Fatalf("Invalid config name %s", configName)
	}
	ActiveRootDir = rootDir
	logrus.Debugf("Run ActiveServerConfig %v ActiveRootDir %s", ActiveServerConfig, ActiveRootDir)

	engine.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	engine.GET(fmt.Sprintf("%s/*rscPath", rscRoot), getContentOrList)
	engine.HEAD(fmt.Sprintf("%s/*rscPath", rscRoot), statContent)
	engine.PUT(fmt.Sprintf("%s/*rscPath", rscRoot), putContentOrMkdir)
	engine.Run(addr)
}

func getContentOrList(c *gin.Context) {
	c.Set("cabri.rscPath", c.Param("rscPath"))
	if strings.HasSuffix(c.Param("rscPath"), "/") {
		ActiveServerConfig.ListFunc(c)
	} else {
		ActiveServerConfig.GetContentFunc(c)
	}
}

func statContent(c *gin.Context) {
	c.Set("cabri.rscPath", c.Param("rscPath"))
	if strings.HasSuffix(c.Param("rscPath"), "/") {
		ActiveServerConfig.StatDirFunc(c)
	} else {
		ActiveServerConfig.StatContentFunc(c)
	}
}

func putContentOrMkdir(c *gin.Context) {
	c.Set("cabri.rscPath", c.Param("rscPath"))
	if strings.HasSuffix(c.Param("rscPath"), "/") {
		ActiveServerConfig.MkdirFunc(c)
	} else {
		ActiveServerConfig.PutContentFunc(c)
	}
}
