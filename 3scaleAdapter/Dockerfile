FROM golang:1.10 AS build

ADD . /go/src/github.com/3scale/istio-integration/3scaleAdapter
RUN go get -u github.com/golang/dep/cmd/dep
WORKDIR /go/src/github.com/3scale/istio-integration/3scaleAdapter
RUN dep ensure -v && go build -o /tmp/3scaleAdapter cmd/main.go

FROM centos
WORKDIR /app
COPY --from=build /tmp/3scaleAdapter /app/
ENV THREESCALE_LISTEN_ADDR 3333
ENTRYPOINT ./3scaleAdapter

