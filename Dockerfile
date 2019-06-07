FROM registry.access.redhat.com/ubi8/ubi-minimal AS build

ENV GOPATH=/go
ARG BUILDDIR="/tmp/3scale-istio-adapter"

RUN microdnf update --nodocs -y \
 && microdnf install --nodocs -y findutils git go-toolset make perl-Digest-SHA \
 && microdnf clean all -y \
 && rm -rf /var/cache/yum

WORKDIR "${BUILDDIR}"

ARG VERSION=

ADD . "${BUILDDIR}"
RUN PATH="${PATH}:${GOPATH//://bin:}/bin" \
 && if test "x${VERSION}" = "x"; then \
      VERSION="$(git describe --dirty --tags || true)" ; \
    fi \
 && make VERSION="${VERSION:? *** No VERSION could be derived, please specify it}" \
      build-adapter build-cli

FROM registry.access.redhat.com/ubi8/ubi-minimal

ARG BUILDDIR="/tmp/3scale-istio-adapter"

WORKDIR /app
COPY --from=build "${BUILDDIR}/_output/3scale-istio-adapter" /app/
COPY --from=build "${BUILDDIR}/_output/3scale-config-gen" /app/
ENV THREESCALE_LISTEN_ADDR 3333
EXPOSE 3333
EXPOSE 8080

CMD ./3scale-istio-adapter

