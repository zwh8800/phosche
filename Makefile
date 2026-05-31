.PHONY: build build-frontend docker-build clean test test-race run

build:
	go build -o phosche ./cmd/phosche/

build-frontend:
	cd web && npm ci && npm run build

docker-build:
	docker build -t phosche .

test:
	go test ./...

test-race:
	go test -race ./...

clean:
	rm -f phosche
	rm -rf web/dist

run:
	go run ./cmd/phosche/ -config config.yaml
