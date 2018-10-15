#!/bin/bash
set -exuo pipefail
IFS=$'\n\t'

START_DEPENDENCIES=${1:-Yes}

if [ "${START_DEPENDENCIES}" = "Yes" ]; then

    killMixerServerOn=""
    killAdapterOn=""

    function clean() {
        kill -SIGTERM ${killMixerServerOn}
        kill -SIGTERM ${killAdapterOn}
        sleep 2;
    }

    trap clean EXIT

    if ! [ -x "$(command -v mixs)" ]; then
        echo 'Error: mixer server is not installed' >&2
        exit 1
    fi

    if ! [ -x "$(command -v mixc)" ]; then
        echo 'Error: mixer client is not installed' >&2
        exit 1
    fi

    mixs server --configStoreURL=fs://"$(pwd)"/testdata &
    killMixerServerOn=$!
    sleep 3;

    "$(pwd)"/_output/3scale-adapter-integration-bin -test.coverprofile="$(pwd)/_output/integration.cov" &
    killAdapterOn=$!
    sleep 3;

fi

# Ok requests
echo -e "HTTP/1.1 200 OK\r\n\r\n" | netcat -l -p 8090 > /dev/null &
sleep 3;
mixc check --stringmap_attributes request.headers="user-key: HEADER_VALID" \
           -s request.path="/thispath?user_key=VALIDKEY",request.method="get" | grep "Check status was OK" > /dev/null || exit 1
echo "Status 200 test passed"

echo -e "HTTP/1.1 202 ACCEPTED\r\n\r\n" | netcat -l -p 8090 > /dev/null &
sleep 3;
mixc check --stringmap_attributes request.headers="user-key: HEADER_VALID" \
           -s request.path="/thispath?user_key=VALIDKEY",request.method="get" | grep "Check status was OK" > /dev/null || exit 1
echo "Status 202 test passed"


# Forbidden request.
echo -e "HTTP/1.1 403 FORBIDDEN\r\n\r\n" | netcat -l -p 8090 > /dev/null &
sleep 3;
mixc check --stringmap_attributes request.headers="user-key: HEADER_INVALID" \
           -s request.path="/thepath?user_key=INVALIDKEY",request.method="get" | grep "Check status was PERMISSION_DENIED" > /dev/null || exit 1
echo "Status 403 test passed"

