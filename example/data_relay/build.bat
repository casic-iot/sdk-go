SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64

go build -tags netgo -v -o test1 main.go