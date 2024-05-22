BINARY_NAME=discord-minecraft-server-log

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -v -o ${BINARY_NAME}

build-docker:
	docker build . -t ${BINARY_NAME}:latest
