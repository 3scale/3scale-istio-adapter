#!/usr/bin/env bash

# A set of utility functions for working with Istio
# Calls to kubectl expects KUBECONFIG environment variable to be set correctly.


script_dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
root_dir=$(dirname "${script_dir}")
output_dir="${root_dir}"/_output
sample_dir="${root_dir}"/istio/samples


# get_istio_version attempts to parse the istio version from Gopkg.toml if not set.
# If parsing fails, default to setting to the latest version.
function get_istio_version() {
    if [[ "x${ISTIO_VERSION}" = "x" ]] ; then
        # not using regex here because of some compatibility issues across different OS
        ISTIO_VERSION=$(grep -A 1 "istio.io/istio" ${root_dir}/Gopkg.toml | grep "version" | cut -d'"' -f 2 | tr -d "=<>~^")
    fi

    if [[ "x${ISTIO_VERSION}" = "x" ]] ; then
      echo "Unable to parse Istio version from dependencies - attempting to use latest version"
      ISTIO_VERSION=$(curl -L -s https://api.github.com/repos/istio/istio/releases/latest | \
                      grep tag_name | sed "s/ *\"tag_name\": *\"\\(.*\\)\",*/\\1/")
    fi

    if [[ "x${ISTIO_VERSION}" = "x" ]] ; then
      printf "Unable to get latest Istio version. Set ISTIO_VERSION env var and re-run. For example: export ISTIO_VERSION=1.1.11"
      exit;
    fi

    printf ${ISTIO_VERSION}
}


# get_istio downloads specified Istio source
function get_istio() {
    version=$(get_istio_version)

    OS="$(uname)"
    if [[ "x${OS}" = "xDarwin" ]] ; then
      OSEXT="osx"
    else
      OSEXT="linux"
    fi

    NAME="istio-${version}"
    URL="https://github.com/istio/istio/releases/download/${version}/istio-${version}-${OSEXT}.tar.gz"
    if ! [[ -d ${output_dir}/${NAME} ]]; then
      printf "Downloading %s from %s ..." "$NAME" "$URL"
      mkdir - p "${output_dir}" && cd "${output_dir}" && curl -L "$URL" | tar xz
    fi
}

function build_istio_src_dir() {
        istio_version=$(get_istio_version)
        ISTIO_SRC=${output_dir}/istio-${istio_version}
        printf ${ISTIO_SRC}
}

# install_istio relies on helm to install a default istio deployment
# accepts path to istio source as arg - if not set attempts to build path from environment
function install_istio() {
    ISTIO_SRC=$1
    if [[ -z "${ISTIO_SRC}" ]]
    then
        ISTIO_SRC=$(build_istio_src_dir)
    fi

    kubectl create ns istio-system || true
    helm template ${ISTIO_SRC}/install/kubernetes/helm/istio-init --name istio-init --namespace istio-system | kubectl apply -f -

    # we need to wait for the custom resources installed by the previous command to be committed to the k8-api
    crd_target_count=$(find ${ISTIO_SRC}/install/kubernetes/helm/istio-init/files -name "crd-[0-9]*" | xargs grep "kind: CustomResourceDefinition" | wc -l)

    ready=false
    for i in {1..12};do
        crd_count=$(kubectl get crds -n istio-system | grep 'istio.io\|certmanager.k8s.io' | wc -l)
        if [[ ${crd_count} -gt $(( ${crd_target_count} - 1 )) ]] ;then ready=true && break; fi
        sleep 5
    done;

    if [[ ${ready} != "true" ]]; then echo "Required CRD missing" &&  exit 1; fi


    # here we need to disable validation/injector functionality which use webhooks since k3s does not yet support that
    # we are re-enabling policies since this is required for the 3scale adapter to work correctly
    helm template ${ISTIO_SRC}/install/kubernetes/helm/istio --name istio --namespace istio-system \
        --set global.configValidation=false --set sidecarInjectorWebhook.enabled=false \
        --set global.disablePolicyChecks=false | kubectl apply -f -

    kubectl rollout status deploy/istio-ingressgateway -n istio-system
}


function deploy_httpbin() {
    kubectl create namespace httpbin || true

    kubectl -n istio-system get configmap istio-sidecar-injector -o=jsonpath='{.data.config}' > ${output_dir}/inject-config.yaml
    kubectl -n istio-system get configmap istio -o=jsonpath='{.data.mesh}' > ${output_dir}/mesh-config.yaml

    TARGET_DIRECTORY=$(build_istio_src_dir)
    ${TARGET_DIRECTORY}/bin/istioctl kube-inject \
        --injectConfigFile ${output_dir}/inject-config.yaml \
        --meshConfigFile ${output_dir}/mesh-config.yaml \
        --filename ${sample_dir}/httpbin/httpbin.yaml \
        --output ${output_dir}/injected.yaml

    kubectl apply -f ${output_dir}/injected.yaml --namespace httpbin
    kubectl apply -f ${sample_dir}/httpbin/httpbin-gateway.yaml --namespace httpbin
    kubectl rollout status deploy/httpbin --namespace httpbin
}
