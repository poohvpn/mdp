
up:
	go get -u github.com/poohvpn/pooh
	go mod tidy

echo-server:
	mkdir -p bin
	go build -o bin/echo-server ./example/echo-server
	sudo setcap cap_net_raw+ep ./bin/echo-server

echo-client:
	mkdir -p bin
	go build -o bin/echo-client ./example/echo-client
	sudo setcap cap_net_raw+ep ./bin/echo-client
