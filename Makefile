BINARY := claude-sidebar

.PHONY: build install clean

build:
	go build -o $(BINARY) ./cmd/claude-sidebar/

install:
	go install ./cmd/claude-sidebar/

clean:
	rm -f $(BINARY)
