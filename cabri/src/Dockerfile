FROM golang:1.14
WORKDIR /go/src/app

RUN go get -u github.com/aws/aws-sdk-go/...
RUN go get -u github.com/sirupsen/logrus
RUN go get -u github.com/gin-gonic/gin
RUN go get -u github.com/toorop/gin-logrus

COPY cabri /usr/local/go/src/cabri
COPY server server
COPY examples examples
RUN go build server/main.go && mv main /cabri-server
RUN go build examples/synchro/main.go && mv main /cabri-synchro-client
