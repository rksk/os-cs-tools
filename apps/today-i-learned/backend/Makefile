BINARY := server

.PHONY: all vet test build clean

all: build

vet:
	go vet ./...

# The store tests also run against MySQL when TIL_TEST_DATABASE_DSN is set
# (see internal/store/store_test.go); otherwise they are skipped.
test: vet
	go test -race ./...

build: test
	go build -o $(BINARY) ./cmd/server

clean:
	rm -f $(BINARY)
