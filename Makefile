# Generate Go code from the .proto files (needs protoc, protoc-gen-go, protoc-gen-go-grpc)
proto:
	mkdir -p gen/itempb gen/claimpb
	protoc --proto_path=proto \
		--go_out=. --go_opt=module=lostfound \
		--go-grpc_out=. --go-grpc_opt=module=lostfound \
		proto/item.proto proto/claim.proto

up:
	docker compose up --build

down:
	docker compose down

reset:
	docker compose down -v
