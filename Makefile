# Cross-compilation Makefile for Go binaries
# Targets: darwin/arm64 (ARM Mac) and linux/amd64 (HPC cluster)
# Run: make -j binaries

BINARIES = adj2edge cap-import cite-detector-moml lm-diagnostic
BIN_DIR = bin

DARWIN_TARGETS = $(foreach bin,$(BINARIES),$(BIN_DIR)/darwin-arm64/$(bin))
LINUX_TARGETS = $(foreach bin,$(BINARIES),$(BIN_DIR)/linux-amd64/$(bin))

.PHONY: binaries clean test vet

binaries: $(DARWIN_TARGETS) $(LINUX_TARGETS)

$(BIN_DIR)/darwin-arm64/%: FORCE
	@mkdir -p $(dir $@)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags '-s -w' -o $@ ./$*

$(BIN_DIR)/linux-amd64/%: FORCE
	@mkdir -p $(dir $@)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o $@ ./$*

FORCE:

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf $(BIN_DIR)/*
