rm -f cnsctl-darwin cnsctl-linux && GOOS=darwin go build -o cnsctl-darwin main.go && GOOS=linux go build -o cnsctl-linux main.go
