FROM golang:1.10 AS build

ENV WORKDIR=/go/src/github.com/3scale/3scale-istio-adapter

ADD . ${WORKDIR}
WORKDIR ${WORKDIR}

RUN go build -race -gcflags "all=-N -l" -o /tmp/3scale-istio-adapter cmd/server/main.go


FROM philipgough/dlv:centos as debugger

FROM centos

ENV THREESCALE_LISTEN_ADDR 3333

WORKDIR /app

COPY --from=build /tmp/3scale-istio-adapter /app/
COPY --from=debugger /go/bin/dlv /app/

EXPOSE 3333
EXPOSE 8080
EXPOSE 40000

CMD ["/app/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/app/3scale-istio-adapter"]

