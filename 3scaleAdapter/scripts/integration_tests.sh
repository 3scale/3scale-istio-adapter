#!/bin/bash
set -exuo pipefail
IFS=$'\n\t'

# Expects the adapter and mixer to be running:
# make adapter
# make mixer

# Ok request

echo -e "HTTP/1.1 200 OK\r\n\r\n" | netcat -l -p 8090 > /dev/null &

mixc check --stringmap_attributes request.headers="user-key: HEADER_VALID" \
           -s request.path="/thispath?user_key=VALIDKEY",request.method="get" | grep "Check status was OK" > /dev/null|| exit 1

echo -e "HTTP/1.1 202 ACCEPTED\r\n\r\n" | netcat -l -p 8090 > /dev/null &

mixc check --stringmap_attributes request.headers="user-key: HEADER_VALID" \
           -s request.path="/thispath?user_key=VALIDKEY",request.method="get" | grep "Check status was OK" > /dev/null|| exit 1


# Forbidden request.

echo -e "HTTP/1.1 403 FORBIDDEN\r\n\r\n" | netcat -l -p 8090 > /dev/null &

mixc check --stringmap_attributes request.headers="user-key: HEADER_INVALID" \
           -s request.path="/thepath?user_key=INVALIDKEY",request.method="get" | grep "Check status was PERMISSION_DENIED" > /dev/null || exit 1

