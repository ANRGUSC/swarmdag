FROM alpine:3.9
MAINTAINER Jason Tran <jasontra@usc.edu>

# Dockerfile for testing swarmdag without using CORE (LXCs)

# Do not run this Dockerfile directly. Please see top level Makefile.

# OS environment setup
# Set user right away for determinism, create directory for persistence and give our user ownership
# jq and curl used for extracting `pub_key` from private validator while
# deploying tendermint with Kubernetes. It is nice to have bash so the users
# could execute bash commands. Also, cleanup is easier without using root.
RUN apk update && \
    apk upgrade && \
    apk --no-cache add curl jq bash

# When using LXCs in CORE, each container has access to the host machine's 
# filesystem. This Docker container will pretend to have a $GOPATH for cross
# compatibility.
ENV GOPATH /fakego

VOLUME [ $GOPATH/src/github.com/ANRGUSC/swarmdag/build/ ]
WORKDIR $GOPATH/src/github.com/ANRGUSC/swarmdag/build

# p2p and rpc port
EXPOSE 26656 26657

# wrapper.sh should be mounted into the container at the workdir
ENTRYPOINT ./wrapper.sh
STOPSIGNAL SIGTERM
