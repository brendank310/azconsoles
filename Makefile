# Makefile for azconsoles project

# Default destination directory for binaries
DESTDIR ?= bin

# Project name
PROJECT_NAME = azconsoles

# Go build options
GOFLAGS =

# List all .go files in cmd/azconsoles and strip the .go extension to get the binary names
BINARIES = $(patsubst cmd/azconsoles/%.go,%,$(wildcard cmd/azconsoles/*.go))

# Define targets for each binary
all: $(BINARIES)

$(BINARIES):
	@echo "Building $@..."
	@mkdir -p $(DESTDIR)
	@go build $(GOFLAGS) -o $(DESTDIR)/$@ ./cmd/$(PROJECT_NAME)/$@.go

install: all
	@echo "Installing binaries to $(DESTDIR)..."
	@mkdir -p $(DESTDIR)
	@for binary in $(BINARIES); do \
		cp $(DESTDIR)/$$binary $(DESTDIR)/$$binary; \
	done
	@echo "Binaries installed to $(DESTDIR)."

clean:
	@echo "Cleaning up..."
	@rm -rf $(DESTDIR)
	@echo "Cleanup complete."

.PHONY: all install clean

