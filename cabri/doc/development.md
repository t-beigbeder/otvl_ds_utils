# cabri development documentation

[documentation index](../../README.md)

## Setup go dependencies

    $ go get -u golang.org/x/tools/...
    $ go get -u github.com/aws/aws-sdk-go/...
    $ go get -u github.com/sirupsen/logrus
    $ go get -u github.com/gin-gonic/gin
    $ go get -u github.com/toorop/gin-logrus

## Build binaries using docker

The Dockerfile will build the server and a client example synchronizing S3 objects to a Filesystem.

    $ cd cabri/src
    $ docker build --pull -t cabri_build:dev001 .
    $ docker run --rm cabri_build:dev001 cat /cabri-server > ~/bin/cabri-server
    $ docker run --rm cabri_build:dev001 cat /cabri-synchro-client > ~/bin/cabri-synchro-client
    $ chmod ugo+x ~/bin/cabri-*


