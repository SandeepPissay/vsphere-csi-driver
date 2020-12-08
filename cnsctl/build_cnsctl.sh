go build -o cnsctl main.go && mv cnsctl $GOPATH/bin && echo "Built cnsctl and installed it." && exit
echo "Build failed!"
