.PHONY: build test fmt vet clean demo

build:
	go build -o bin/contractgate ./cmd/contractgate

test:
	go test ./... -count=1

test-race:
	go test ./... -race -count=1

fmt:
	gofmt -l .

vet:
	go vet ./...

demo: build
	./bin/contractgate demo

clean:
	rm -rf bin/ *.db
