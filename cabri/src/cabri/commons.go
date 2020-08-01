package cabri

import (
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
)

type ServerConfig struct {
	GetContentFunc  gin.HandlerFunc
	StatContentFunc gin.HandlerFunc
	StatDirFunc     gin.HandlerFunc
	ListFunc        gin.HandlerFunc
	PutContentFunc  gin.HandlerFunc
	MkdirFunc       gin.HandlerFunc
}

type StatContent struct {
	LastModified time.Time
	Size         int64
	Checksum     string
}

var ServerConfigMap = map[string]ServerConfig{
	"S3Read": {
		GetContentFunc:  S3GetContent,
		StatContentFunc: S3StatContent,
		StatDirFunc:     NotImplementedFunc,
		ListFunc:        S3List,
		PutContentFunc:  NotImplementedFunc,
		MkdirFunc:       NotImplementedFunc,
	},
	"FSWrite": {
		GetContentFunc:  FSGetContent,
		StatContentFunc: FSStatContent,
		StatDirFunc:     FSStatDir,
		ListFunc:        FSList,
		PutContentFunc:  FSPutContent,
		MkdirFunc:       FSMkdir,
	},
}

func Error(c *gin.Context, msg string, err error, status int) {
	logrus.Errorf("%s: %v\n", msg, err)
	if status == 0 {
		status = http.StatusInternalServerError
	}
	http.Error(c.Writer, msg, status)
}

func GetContentError(c *gin.Context, path string, err error, status int) {
	Error(c, fmt.Sprintf("get content %s", path), err, status)
}

func ListError(c *gin.Context, path string, err error, status int) {
	Error(c, fmt.Sprintf("list %s", path), err, status)
}

func StatContentError(c *gin.Context, path string, err error, status int) {
	Error(c, fmt.Sprintf("stat content %s", path), err, status)
}

func StatDirError(c *gin.Context, path string, err error, status int) {
	Error(c, fmt.Sprintf("stat dir %s", path), err, status)
}

func PutContentError(c *gin.Context, path string, err error, status int) {
	Error(c, fmt.Sprintf("put content %s", path), err, status)
}

func MkdirError(c *gin.Context, path string, err error, status int) {
	Error(c, fmt.Sprintf("mkdir %s", path), err, status)
}

func NotImplementedFunc(c *gin.Context) {
	logrus.Debugf("NotImplementedFunc %s", c.Keys["cabri.rscPath"].(string))
	Error(c, c.Keys["cabri.rscPath"].(string), fmt.Errorf("not yet implemented"), 0)
}
