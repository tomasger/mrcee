protogen:
	mkdir -p gen/mrcee/v1
	protoc --proto_path=proto --go_out=gen/mrcee/v1 --go_opt=paths=source_relative \
		--go-grpc_out=gen/mrcee/v1 --go-grpc_opt=paths=source_relative mrcee.proto

mrcee:
	go build -o mrcee ./cmd/mrcee

mrcee-install:
	go install ./cmd/mrcee

server:
	go build -o mrcee-server ./cmd/server
