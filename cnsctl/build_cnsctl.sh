#!/bin/bash

# darwin specific build command
# GOOS=darwin go build -o cnsctl main.go
set -x
# debug build 
go build -o cnsctl main.go && mv cnsctl "$GOPATH"/bin && echo "Built cnsctl and installed it." && exit 0
# release build 
#go build -ldflags "-s -w" -o cnsctl main.go && mv cnsctl "$GOPATH"/bin && echo "Built cnsctl and installed it." && exit 0
echo "Build failed!" && exit 1
