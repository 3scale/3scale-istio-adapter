FROM golang:1.10 AS build

ARG VERSION="undefined"
ENV WORKDIR=/go/src/github.com/3scale/3scale-istio-adapter

ADD . ${WORKDIR}
WORKDIR ${WORKDIR}

RUN go get -u github.com/golang/dep/cmd/dep && \
    make build-adapter VERSION=${VERSION} && \
    make build-cli VERSION=${VERSION}

FROM centos

WORKDIR /app
COPY --from=build /go/src/github.com/3scale/3scale-istio-adapter/_output/3scale-istio-adapter /app/
COPY --from=build /go/src/github.com/3scale/3scale-istio-adapter/_output/3scale-config-gen /app/
ENV THREESCALE_LISTEN_ADDR 3333
EXPOSE 3333
EXPOSE 8080
ENTRYPOINT ./3scale-istio-adapter

