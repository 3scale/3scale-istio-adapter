FROM golang:1.10 AS build

ENV WORKDIR=/go/src/github.com/3scale/istio-integration/3scaleAdapter

ADD . ${WORKDIR}
WORKDIR ${WORKDIR}

RUN go get -u github.com/golang/dep/cmd/dep && \
    dep ensure -v && \
    go build -o /tmp/3scaleAdapter cmd/main.go

FROM centos

WORKDIR /app
COPY --from=build /tmp/3scaleAdapter /app/
ENV THREESCALE_LISTEN_ADDR 3333
EXPOSE 3333
EXPOSE 8080
ENTRYPOINT ./3scaleAdapter

