build:
    go build ./...

install:
    go build -o gc ./cmd/git-ctx
    mv gc ~/.local/bin/gc

test:
    go test ./...

test-verbose:
    go test ./... -v
