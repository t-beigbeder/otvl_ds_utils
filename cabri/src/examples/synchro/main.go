package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var debug = false
var mu sync.Mutex
var currentDirs = make(map[string]bool)
var currentContents = make(map[string]bool)
var maxOutstanding = 5

func usage() {
	log.Fatalf("Incorrect flags please read the documentation")
}

func main() {
	var fDebug = flag.Bool("debug", false, "Displays debug messages and run gin in debug mode")
	var sourceUrl = flag.String("source-url", "", "Source URL")
	var targetUrl = flag.String("target-url", "", "Target URL")
	flag.Parse()
	if *sourceUrl == "" {
		log.Fatalf("Empty source-url, please read the documentation")
	}
	if *sourceUrl == "" {
		log.Fatalf("Empty target-url, please read the documentation")
	}

	debug = *fDebug
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}
	logrus.Info("synchro: main: started")
	logrus.Debug("synchro: main: see if we are in debug mode")
	runSynchro(*sourceUrl, *targetUrl)
	return
}

func setCurrent(dir string, content string) {
	mu.Lock()
	defer mu.Unlock()
	if dir != "" {
		currentDirs[dir] = true
	} else {
		currentContents[content] = true
	}
}

func clearCurrent(dir string, content string) {
	mu.Lock()
	defer mu.Unlock()
	if dir != "" {
		delete(currentDirs, dir)
	} else {
		delete(currentContents, content)
	}
}

func getCurrentDir(dir string) bool {
	mu.Lock()
	defer mu.Unlock()
	return currentDirs[dir]
}

func runSynchro(sourceUrl string, targetUrl string) {
	entriesChan := make(chan []string, maxOutstanding)
	currentChan := make(chan string)

	logrus.Debugf("runSynchro %s %s", sourceUrl, targetUrl)

	for i := 0; i < maxOutstanding; i++ {
		go listConsumer(sourceUrl, targetUrl, entriesChan, currentChan)
	}

	entriesChan <- []string{"/"}

	for {
		entries := <-entriesChan
		logrus.Debugf("runSynchro len entries %d", len(entries))

		for _, path := range entries {
			if path[len(path)-1] == '/' {
				setCurrent(path, "")
			}
			logrus.Debugf("runSynchro currentChan %s", currentChan)
			currentChan <- path
		}

		logrus.Debugf("runSynchro len entriesChan %d", len(entriesChan))
		count := 0
		for len(entriesChan) == 0 {
			time.Sleep(100 * time.Millisecond)
			count += 1
			if count == 600 {
				break
			}
		}
		if count >= 600 {
			break
		}
	}
	logrus.Debugf("runSynchro %s %s exiting", sourceUrl, targetUrl)

}

func listConsumer(sourceUrl string, targetUrl string, entriesChan chan []string, currentChan chan string) {
	for {
		path := <-currentChan
		logrus.Debugf("listConsumer %s", path)
		if path[len(path)-1] == '/' {
			setCurrent(path, "")
			defer clearCurrent(path, "")
			waitForParentDir(path)
			entries := synchroDir(sourceUrl, targetUrl, path)
			clearCurrent(path, "")
			entriesChan <- entries
		} else {
			setCurrent("", path)
			defer clearCurrent("", path)
			waitForParentDir(path)
			synchroContent(sourceUrl, targetUrl, path)
		}
		if debug {
			logrus.Debugf("listConsumer %s sleep", path)
			time.Sleep(5 * time.Second)

		}
	}
}

func urlPrefix(url string) string {
	pe := strings.Split(url, "/") // http://dxpydk:8080/s3mcab/test-bk-01
	return pe[len(pe)-1]
}

func waitForParentDir(path string) {
	if path == "/" {
		return
	}
	pe := strings.Split(path, "/")
	if pe[len(pe)-1] == "" {
		pe = pe[0 : len(pe)-1]
	}
	parent := strings.Join(pe[0:len(pe)-1], "/") + "/"
	logrus.Debugf("waitForParentDir %s -> %s parent in queue %v", path, parent, getCurrentDir(parent))
	for getCurrentDir(parent) {
		time.Sleep(time.Second)
		logrus.Debugf("waitForParentDir %s -> %s parent in queue %v", path, parent, getCurrentDir(parent))
	}
}

func synchroDir(sourceUrl string, targetUrl string, path string) (entries []string) {
	var req *http.Request
	var resp *http.Response
	var err error
	var exists bool

	logrus.Debugf("synchroDir %s", path)

	statUrl := fmt.Sprintf("%s%s", targetUrl, path)
	resp, err = http.Head(statUrl)
	if err != nil {
		log.Printf("synchroDir: head: %s error %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		logrus.Debugf("synchroDir %s exists", path)
		exists = true
	}

	if !exists {
		client := &http.Client{}
		putUrl := fmt.Sprintf("%s%s?recursive", targetUrl, path)
		req, err = http.NewRequest(http.MethodPut, putUrl, strings.NewReader(""))
		if err != nil {
			log.Printf("synchroDir: put: %s error %v", path, err)
			return
		}
		logrus.Debugf("synchroDir %s DO %v", path, req)

		resp, err = client.Do(req)
		if err != nil {
			log.Printf("synchroDir: put: %s error %v", path, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("synchroDir: put: %s error %v", path, fmt.Errorf("status %d", resp.StatusCode))
			return
		}
		log.Printf("mkdir %s", path)
	}

	getUrl := fmt.Sprintf("%s%s", sourceUrl, path)
	resp, err = http.Get(getUrl)
	if err != nil {
		log.Printf("synchroDir: get: %s error %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("synchroDir: get: %s error %v", path, fmt.Errorf("status %d", resp.StatusCode))
		return
	}
	rd := bufio.NewReader(resp.Body)
	for {
		line, err := rd.ReadString('\n')
		logrus.Debugf("synchroDir ReadString line %s err %s", line, err)

		if err == io.EOF || line == "\n" {
			break
		}
		if err != nil {
			log.Printf("synchroDir: read: %s error %v", path, err)
			return
		}
		logrus.Debugf("synchroDir ReadString entries append %s (%s)", line[len(urlPrefix(sourceUrl))+1:len(line)-1], urlPrefix(sourceUrl))
		entries = append(entries, line[len(urlPrefix(sourceUrl))+1:len(line)-1])
		// listChan <- line[len(urlPrefix(sourceUrl))+1 : len(line)-1]
	}
	return
}

func synchroContent(sourceUrl string, targetUrl string, path string) {
	var req *http.Request
	var resp *http.Response
	var err error
	var targetCs string

	logrus.Debugf("synchroContent %s", path)

	statUrl := fmt.Sprintf("%s%s", targetUrl, path)
	resp, err = http.Head(statUrl)
	if err != nil {
		log.Printf("synchroContent: head target: %s error %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		targetCs = resp.Header.Get("Checksum")
		logrus.Debugf("synchroContent %s exists Checksum %s", path, targetCs)
	}

	if targetCs != "" {
		statUrl := fmt.Sprintf("%s%s", sourceUrl, path)
		resp, err = http.Head(statUrl)
		if err != nil {
			log.Printf("synchroContent: head source: %s error %v", path, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if resp.Header.Get("Checksum") == targetCs {
				logrus.Debugf("synchroContent %s exists with same Checksum %s", path, targetCs)
				return
			}
		}
	}

	getUrl := fmt.Sprintf("%s%s", sourceUrl, path)
	resp, err = http.Get(getUrl)
	if err != nil {
		log.Printf("synchroContent: get: %s error %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("synchroContent: get: %s error %v", path, fmt.Errorf("status %d", resp.StatusCode))
		return
	}

	var tmpfileW *os.File
	if tmpfileW, err = ioutil.TempFile("", "cabri*"); err != nil {
		log.Printf("synchroContent: TempFile: %s error %v", path, err)
		return
	}
	defer os.Remove(tmpfileW.Name())
	defer tmpfileW.Close()
	if _, err = io.Copy(tmpfileW, resp.Body); err != nil {
		log.Printf("synchroContent: Copy: %s error %v", path, err)
		return
	}
	tmpfileW.Close()
	var tmpfileR *os.File
	if tmpfileR, err = os.Open(tmpfileW.Name()); err != nil {
		log.Printf("synchroContent: open TempFile: %s error %v", path, err)
	}
	defer tmpfileR.Close()

	client := &http.Client{}
	putUrl := fmt.Sprintf("%s%s", targetUrl, path)
	req, err = http.NewRequest(http.MethodPut, putUrl, tmpfileR)
	if err != nil {
		log.Printf("synchroContent: put: %s error %v", path, err)
		return
	}
	req.Header.Add("Last-Modified", resp.Header.Get("Last-Modified"))
	logrus.Debugf("synchroContent %s DO %v", path, req)

	resp, err = client.Do(req)
	if err != nil {
		log.Printf("synchroContent: put: %s error %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("synchroContent: put: %s error %v", path, fmt.Errorf("status %d", resp.StatusCode))
		return
	}
	log.Printf("put content %s", path)
}
