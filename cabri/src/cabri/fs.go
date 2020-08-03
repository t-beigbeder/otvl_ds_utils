package cabri

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func FSGetContent(c *gin.Context) {
	logrus.Debugf("FSGetContent %s", c.Keys["cabri.rscPath"].(string))
	fsGetContent(c, c.Keys["cabri.rscPath"].(string), "")
}

func fsGetContent(c *gin.Context, rscPath string, checksum string) {
	path := fmt.Sprintf("%s%s", ActiveRootDir, rscPath)
	logrus.Debugf("fsGetContent %s", path)
	var f *os.File
	var err error
	if f, err = os.Open(path); err != nil {
		if checksum != "" {
			w := c.Writer
			w.WriteHeader(http.StatusNotFound)
			return
		}
		GetContentError(c, path, err, http.StatusNotFound)
		return
	}
	defer f.Close()
	var info os.FileInfo // IsDir() Size() ModTime()
	if info, err = f.Stat(); err != nil {
		GetContentError(c, path, err, 0)
		return
	}
	if info.IsDir() {
		GetContentError(c, path, fmt.Errorf("is a directory"), http.StatusNotFound)
		return
	}
	if checksum == "" {
		logrus.Debugf("fsGetContent ServeFile on %s", path)
		http.ServeFile(c.Writer, c.Request, path)
	} else {
		var cs string
		if cs, err = GetChecksum(checksum, path); err != nil {
			Error(c, fmt.Sprintf("fsGetContent path %s", path), err, http.StatusBadRequest)
			return
		}
		statContent := &StatContent{
			LastModified: info.ModTime(),
			Size:         info.Size(),
			Checksum:     cs,
		}
		c.Set("cabri.statContent", statContent)
	}
	return
}

func FSStatContent(c *gin.Context) {
	logrus.Debugf("FSStatContent %s", c.Keys["cabri.rscPath"].(string))
	fsGetContent(c, c.Keys["cabri.rscPath"].(string), "sha256")
	if _, exists := c.Get("cabri.statContent"); !exists {
		return
	}
	statContent := c.Keys["cabri.statContent"].(*StatContent)
	w := c.Writer
	SetLastModified(w, statContent.LastModified)
	w.Header().Set("Content-Length", strconv.FormatInt(statContent.Size, 10))
	w.Header().Set("Checksum", statContent.Checksum)
	w.WriteHeader(http.StatusOK)
}

func FSStatDir(c *gin.Context) {
	logrus.Debugf("FSStatDir %s", c.Keys["cabri.rscPath"].(string))
	path := fmt.Sprintf("%s%s", ActiveRootDir, c.Keys["cabri.rscPath"].(string))
	var f *os.File
	var err error
	if f, err = os.Open(path); err != nil {
		w := c.Writer
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer f.Close()
	var info os.FileInfo // IsDir() Size() ModTime()
	if info, err = f.Stat(); err != nil {
		StatDirError(c, path, err, 0)
		return
	}
	if !info.IsDir() {
		GetContentError(c, path, fmt.Errorf("is not a directory"), http.StatusNotFound)
		return
	}

	w := c.Writer
	SetLastModified(w, info.ModTime())
	w.WriteHeader(http.StatusOK)
}

func FSList(c *gin.Context) {
	rscPath := c.Keys["cabri.rscPath"].(string)
	path := fmt.Sprintf("%s%s", ActiveRootDir, rscPath)
	logrus.Debugf("FSList %s", path)
	var f *os.File
	var err error
	if f, err = os.Open(path); err != nil {
		ListError(c, path, err, http.StatusNotFound)
		return
	}
	defer f.Close()
	var info os.FileInfo // IsDir() Size() ModTime()
	if info, err = f.Stat(); err != nil {
		ListError(c, path, err, 0)
		return
	}
	if !info.IsDir() {
		ListError(c, path, fmt.Errorf("not a directory"), http.StatusNotFound)
		return
	}
	var infos []os.FileInfo
	if infos, err = f.Readdir(0); err != nil {
		ListError(c, path, err, 0)
		return
	}
	w := c.Writer
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	dNames := make([]string, 0, len(infos))
	fNames := make([]string, 0, len(infos))
	for _, info := range infos {
		if info.IsDir() {
			dNames = append(dNames, info.Name())
		} else {
			fNames = append(fNames, info.Name())
		}
	}
	sort.Strings(dNames)
	sort.Strings(fNames)

	for _, name := range dNames {
		fmt.Fprintf(w, "%s%s/\n", rscPath, name)
	}
	for _, name := range fNames {
		fmt.Fprintf(w, "%s%s\n", rscPath, name)
	}
	fmt.Fprintf(w, "\n")

}

func FSPutContent(c *gin.Context) {
	rscPath := c.Keys["cabri.rscPath"].(string)
	path := fmt.Sprintf("%s%s", ActiveRootDir, rscPath)
	logrus.Debugf("FSPutContent %s", path)
	var f *os.File
	var err error
	if f, err = os.Open(path); err == nil {
		defer f.Close()
		var info os.FileInfo
		if info, err = f.Stat(); err != nil {
			PutContentError(c, path, fmt.Errorf("cannot Stat"), 0)
			return
		}
		if info.IsDir() {
			PutContentError(c, path, fmt.Errorf("is a directory"), http.StatusBadRequest)
			return
		}
		logrus.Debugf("FSPutContent %s already exists", path)
	} else {
		logrus.Debugf("FSPutContent %s created", path)
	}
	if f, err = os.Create(path); err != nil {
		PutContentError(c, path, err, 0)
		return
	}
	defer f.Close()
	var wln int64
	if wln, err = io.Copy(f, c.Request.Body); err != nil {
		PutContentError(c, path, err, 0)
		return
	}
	t, err := http.ParseTime(c.Request.Header.Get("last-modified"))
	if err != nil {
		PutContentError(c, path, err, http.StatusBadRequest)
		return
	}
	logrus.Debugf("FSPutContent %s copied %d bytes mtime %v", path, wln, t)
	f.Close()
	err = os.Chtimes(f.Name(), t, t)
	if err != nil {
		PutContentError(c, path, err, http.StatusBadRequest)
		return
	}

	w := c.Writer
	w.WriteHeader(http.StatusOK)
	return
}

func FSMkdir(c *gin.Context) {
	recursive := false
	rscPath := c.Keys["cabri.rscPath"].(string)
	path := fmt.Sprintf("%s%s", ActiveRootDir, rscPath)
	logrus.Debugf("FSMkdir %s", path)
	_, ok := c.Request.URL.Query()["recursive"]
	if ok {
		recursive = true
	}
	var f *os.File
	var err error
	if f, err = os.Open(path); err == nil {
		defer f.Close()
		var info os.FileInfo
		if info, err = f.Stat(); err != nil {
			MkdirError(c, path, fmt.Errorf("cannot Stat"), 0)
			return
		}
		if !info.IsDir() {
			MkdirError(c, path, fmt.Errorf("is not a directory"), http.StatusBadRequest)
			return
		}
		logrus.Debugf("FSMkdir %s already exists", path)
	} else {
		var err error
		if recursive {
			err = os.MkdirAll(path, 0777)
		} else {
			err = os.Mkdir(path, 0777)
		}
		if err != nil {
			MkdirError(c, path, err, 0)
			return
		}
		logrus.Debugf("FSMkdir %s created", path)
		log.Printf("mkdir %s", path)
	}
	w := c.Writer
	w.WriteHeader(http.StatusOK)
	return
}
