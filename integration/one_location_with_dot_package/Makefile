include ../env.mk

pwd:
	@pwd

clean:
	rm -f ./pb/strings.pb.go
	rm -f ./pb/strings.pb.goclay.go
	rm -f ./pb/strings.go
	rm -f ./pb/to_lower.go
	rm -f ./pb/to_upper.go
	rm -f main

protoc: protoc-build
	protoc --plugin=protoc-gen-goclay=$(GEN_CLAY_BIN) --plugin=protoc-gen-gofast=$(GEN_GOFAST_BIN) -I/usr/local/include:${GRPC_GATEWAY_PATH}/third_party/googleapis:. --gofast_out=plugins=grpc:. --goclay_out=impl=true:. pb/strings.proto

build:
	go build -o main main.go

test: pwd clean protoc build
