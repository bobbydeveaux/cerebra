# Cerebra build targets.
#
# IMPORTANT: always build with the `sqlite_fts5` tag. Without it, both
# `chunks_fts` (codebase search) and `agent_messages_fts` (agent invocation
# search) silently fail to create, falling back to slow LIKE queries.

BINARY := cerebra
PKG    := .
TAGS   := sqlite_fts5

.PHONY: build test vet clean install

build:
	go build -tags "$(TAGS)" -o $(BINARY) $(PKG)

test:
	go test -tags "$(TAGS)" ./...

vet:
	go vet -tags "$(TAGS)" ./...

clean:
	rm -f $(BINARY)

install: build
	cp $(BINARY) $(HOME)/.local/bin/$(BINARY)
