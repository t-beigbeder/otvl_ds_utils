package cabri

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
)

var s3Svc *s3.S3
var mu sync.Mutex

func getS3Svc() {
	if s3Svc == nil {
		mu.Lock()
		defer mu.Unlock()
		if s3Svc == nil {
			sess := session.Must(session.NewSession())
			stsSvc := sts.New(sess)
			callerIdentity, err := stsSvc.GetCallerIdentity(&sts.GetCallerIdentityInput{})
			logrus.Debugf("getS3Svc callerIdentity %v\nerr %v\n", callerIdentity, err)
			s3Svc = s3.New(sess)
			logrus.Debugf("getS3Svc s3Svc %+v\n", *s3Svc)
		}
	}
}

func S3GetContent(c *gin.Context) {
	logrus.Debugf("S3GetContent %s", c.Keys["cabri.rscPath"])
	pe := strings.Split(c.Keys["cabri.rscPath"].(string), "/")
	s3GetContent(c, pe[1], strings.Join(pe[2:], "/"), "")
}

func s3GetContent(c *gin.Context, bucketName string, objectKey string, checksum string) {
	getS3Svc()
	logrus.Debugf("s3GetContent %s %s", bucketName, objectKey)
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	}
	result, err := s3Svc.GetObject(input)
	if err != nil {
		s3GetContentError(c, bucketName, objectKey, err, 0)
		return
	}
	defer result.Body.Close()
	ln := *result.ContentLength
	ext := filepath.Ext(objectKey)
	var tmpfile *os.File
	if tmpfile, err = ioutil.TempFile("", fmt.Sprintf("cabri*%s", ext)); err != nil {
		Error(c, fmt.Sprintf("s3GetContent objectKey %s", objectKey), err, 0)
		return
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()
	var wln int64
	if wln, err = io.Copy(tmpfile, result.Body); err != nil {
		Error(c, fmt.Sprintf("s3GetContent objectKey %s", objectKey), err, 0)
		return
	}
	if wln != ln {
		Error(c, fmt.Sprintf("s3GetContent objectKey %s", objectKey), fmt.Errorf("wln %d != ln %d", wln, ln), 0)
		return
	}
	tmpfile.Close()
	// t, _ := http.ParseTime()
	os.Chtimes(tmpfile.Name(), *result.LastModified, *result.LastModified)
	if checksum == "" {
		logrus.Debugf("s3GetContent ServeFile on %s", tmpfile.Name())
		http.ServeFile(c.Writer, c.Request, tmpfile.Name())
	} else {
		var cs string
		if cs, err = GetChecksum(checksum, tmpfile.Name()); err != nil {
			Error(c, fmt.Sprintf("s3GetContent objectKey %s", objectKey), err, http.StatusBadRequest)
			return
		}
		statContent := &StatContent{
			LastModified: *result.LastModified,
			Size:         *result.ContentLength,
			Checksum:     cs,
		}
		c.Set("cabri.statContent", statContent)
	}
	return
}

func s3GetContentError(c *gin.Context, bucket string, key string, err error, status int) {
	var s3err awserr.RequestFailure
	path := fmt.Sprintf("%s/%s", bucket, key)
	if errors.As(err, &s3err) {
		logrus.Debugf("s3GetContentError awserr.RequestFailure %s err %#v\n", path, err)
		if s3err.StatusCode() == http.StatusNotFound {
			status = http.StatusNotFound
		} else {
			status = http.StatusBadGateway
		}
		GetContentError(c, path, err, status)
		return
	}
	GetContentError(c, path, err, status)
}

func S3List(c *gin.Context) {
	logrus.Debugf("S3List %s", c.Keys["cabri.rscPath"])
	pe := strings.Split(c.Keys["cabri.rscPath"].(string), "/")
	s3List(c, pe[1], strings.Join(append([]string{""}, pe[2:]...), "/"))
}

type s3ListEntry struct {
	key          string
	lastModified time.Time
	size         int64
	isPrefix     bool
}

func s3List(c *gin.Context, bucketName string, prefix string) {
	getS3Svc()
	logrus.Debugf("s3List %+v %s %s", *s3Svc.Client, bucketName, prefix)
	var input *s3.ListObjectsV2Input
	input = &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucketName),
		Delimiter: aws.String("/"),
		Prefix:    aws.String(prefix[1:]),
	}

	done := false
	listByKey := make(map[string]s3ListEntry)
	for !done {
		logrus.Debugf("s3List input b %s p %s c %s", bucketName, prefix, input.ContinuationToken)

		result, err := s3Svc.ListObjectsV2(input)
		logrus.Debugf("s3List res r %v e %v", result, err)

		if err != nil {
			s3ListError(c, bucketName, prefix, err, 0)
			return
		}
		done = !*result.IsTruncated
		for _, content := range result.Contents {
			logrus.Debugf("s3List content %s", *content.Key)
			listByKey[*content.Key] = s3ListEntry{
				key:          *content.Key,
				lastModified: *content.LastModified,
				size:         *content.Size,
			}
		}
		for _, commonPrefix := range result.CommonPrefixes {
			logrus.Debugf("s3List prefix  %s", *commonPrefix.Prefix)
			listByKey[*commonPrefix.Prefix] = s3ListEntry{
				key:      *commonPrefix.Prefix,
				isPrefix: true,
			}
		}
		if !done {
			input.ContinuationToken = result.NextContinuationToken
		}
	}
	if len(listByKey) == 0 {
		s3ListError(c, bucketName, prefix, fmt.Errorf("object with trailing \"/\"?"), http.StatusNotFound)
		return
	}
	w := c.Writer
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	pKeys := make([]string, 0, len(listByKey))
	cKeys := make([]string, 0, len(listByKey))
	for key, entry := range listByKey {
		logrus.Debugf("s3List listByKey k %s e %v", key, entry)
		if entry.isPrefix {
			pKeys = append(pKeys, key)
		} else {
			cKeys = append(cKeys, key)
		}
	}
	sort.Strings(pKeys)
	sort.Strings(cKeys)
	logrus.Debugf("s3List pKeys %v cKeys %v", pKeys, cKeys)

	for _, key := range pKeys {
		fmt.Fprintf(w, "/%s/%s\n", bucketName, key)
	}
	for _, key := range cKeys {
		if key == prefix[1:] {
			continue
		}
		fmt.Fprintf(w, "/%s/%s\n", bucketName, key)
	}
	fmt.Fprintf(w, "\n")
}

func s3ListError(c *gin.Context, bucket string, prefix string, err error, status int) {
	var s3err awserr.RequestFailure
	path := fmt.Sprintf("%s/%s", bucket, prefix)
	if errors.As(err, &s3err) {
		logrus.Debugf("s3ListError awserr.RequestFailure %s err %#v\n", path, err)
		if s3err.StatusCode() == http.StatusNotFound {
			status = http.StatusNotFound
		} else {
			status = http.StatusBadGateway
		}
		ListError(c, path, err, status)
		return
	}
	ListError(c, path, err, status)
}

func S3StatContent(c *gin.Context) {
	logrus.Debugf("S3StatContent %s", c.Keys["cabri.rscPath"])
	pe := strings.Split(c.Keys["cabri.rscPath"].(string), "/")
	s3GetContent(c, pe[1], strings.Join(pe[2:], "/"), "sha256")
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
