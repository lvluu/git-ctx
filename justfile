build:
    go build ./...

install:
    go build -o gc .
    mv gc ~/.local/bin/gc

test:
    go test ./...

test-verbose:
    go test ./... -v
