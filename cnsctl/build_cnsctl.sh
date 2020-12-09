# GOOS specific build command
# GOOS=darwin go build -o cnsctl main.go
go build -o cnsctl main.go && mv cnsctl $GOPATH/bin && echo "Built cnsctl and installed it." && exit
echo "Build failed!" && exit 1
