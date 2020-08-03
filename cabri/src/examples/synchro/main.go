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
var entries = []string{}

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

func pushEntry(id string, newEntries []string) {
	logrus.Debugf("pushEntry %s: %d <- %d", id, len(entries), len(newEntries))
	mu.Lock()
	defer mu.Unlock()
	for _, entry := range newEntries {
		entries = append(entries, entry)
	}
}

func pullEntry(id string) (entry string) {
	logrus.Debugf("pullEntry %s: %d", id, len(entries))
	mu.Lock()
	defer mu.Unlock()
	if len(entries) == 0 {
		return
	}
	entry = entries[0]
	entries = entries[1:len(entries)]
	logrus.Debugf("pullEntry %s: %d -> %s", id, len(entries), entry)
	return
}

func runSynchro(sourceUrl string, targetUrl string) {
	currentChan := make(chan string)

	logrus.Debugf("runSynchro %s %s", sourceUrl, targetUrl)

	for i := 0; i < maxOutstanding; i++ {
		ecId := fmt.Sprintf("EC#%d", i)
		go entryConsumer(ecId, sourceUrl, targetUrl, currentChan)
	}

	id := "RSYN"
	pushEntry(id, []string{"/"})

	for {
		path := ""
		count := 0
		for {
			path = pullEntry(id)
			if path != "" {
				break
			}
			time.Sleep(100 * time.Millisecond)
			count += 1
			if count == 600 {
				break
			}
		}
		if count >= 600 {
			break
		}

		if path[len(path)-1] == '/' {
			setCurrent(path, "")
		}
		logrus.Debugf("runSynchro currentChan <- %s", path)
		currentChan <- path
	}
	logrus.Debugf("runSynchro %s %s exiting", sourceUrl, targetUrl)
}

func entryConsumer(id string, sourceUrl string, targetUrl string, currentChan chan string) {
	for {
		path := <-currentChan
		logrus.Debugf("entryConsumer%s %s", id, path)
		if path[len(path)-1] == '/' {
			setCurrent(path, "")
			defer clearCurrent(path, "")
			waitForParentDir(path)
			entries := synchroDir(id, sourceUrl, targetUrl, path)
			clearCurrent(path, "")
			pushEntry(id, entries)
		} else {
			setCurrent("", path)
			defer clearCurrent("", path)
			waitForParentDir(path)
			synchroContent(id, sourceUrl, targetUrl, path)
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

func synchroDir(id string, sourceUrl string, targetUrl string, path string) (entries []string) {
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
		logrus.Debugf("synchroDir%s %s exists", id, path)
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
		logrus.Debugf("synchroDir%s %s DO %v", id, path, req)

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
		logrus.Debugf("synchroDir%s ReadString line %s err %s", id, line, err)

		if err == io.EOF || line == "\n" {
			break
		}
		if err != nil {
			log.Printf("synchroDir: read: %s error %v", path, err)
			return
		}
		logrus.Debugf("synchroDir%s ReadString entries append %s (%s)", id, line[len(urlPrefix(sourceUrl))+1:len(line)-1], urlPrefix(sourceUrl))
		entries = append(entries, line[len(urlPrefix(sourceUrl))+1:len(line)-1])
	}
	return
}

func synchroContent(id string, sourceUrl string, targetUrl string, path string) {
	var req *http.Request
	var resp *http.Response
	var err error
	var targetCs string

	logrus.Debugf("synchroContent%s %s", id, path)

	statUrl := fmt.Sprintf("%s%s", targetUrl, path)
	resp, err = http.Head(statUrl)
	if err != nil {
		log.Printf("synchroContent: head target: %s error %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		targetCs = resp.Header.Get("Checksum")
		logrus.Debugf("synchroContent%s %s exists Checksum %s", id, path, targetCs)
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
				logrus.Debugf("synchroContent%s %s exists with same Checksum %s", id, path, targetCs)
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
	logrus.Debugf("synchroContent%s %s DO %v", id, path, req)

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
