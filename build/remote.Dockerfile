FROM golang as builder
COPY go.mod go.sum cmd pkg version /mc-robot/
COPY pkg/ /mc-robot/pkg/
COPY cmd/ /mc-robot/cmd/
COPY version/ /mc-robot/version/
WORKDIR /mc-robot
RUN GO111MODULE=on GOOS=linux CGO_ENABLED=0 go build -o build/_output/bin/mc-robot \
  -gcflags "all=-trimpath=$(dirname $PWD);$HOME" \
  -asmflags "all=-trimpath=$(dirname $PWD);$HOME" \
  -ldflags=-buildid= \
  q42/mc-robot/cmd/manager

# See build/Dockerfile
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/mc-robot \
    USER_UID=1001 \
    USER_NAME=mc-robot

# install operator binary
COPY --from=builder /mc-robot/build/_output/bin/mc-robot ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
