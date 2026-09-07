# Cross-compilation Makefile for Go binaries
# Targets: darwin/arm64 (ARM Mac) and linux/amd64 (HPC cluster)
# Run: make -j binaries

BINARIES = adj2edge cap-import cite-detector-moml cite-linker lm-diagnostic
BIN_DIR = bin

DARWIN_TARGETS = $(foreach bin,$(BINARIES),$(BIN_DIR)/darwin-arm64/$(bin))
LINUX_TARGETS = $(foreach bin,$(BINARIES),$(BIN_DIR)/linux-amd64/$(bin))

.PHONY: binaries clean test vet sync-hopper db-up db-down db-status db-schema db-stubs db-maintenance chambers

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

sync-hopper: $(LINUX_TARGETS)
	rsync --delete --checksum -avz --itemize-changes $(BIN_DIR)/linux-amd64/ hopper:~/legal-modernism/bin/
	rsync --delete --checksum -avz --itemize-changes slurm/ hopper:~/legal-modernism/jobs/

# Strip pool_max_conns from the connection string since dbmate doesn't support it
DBMATE_URL := $(shell echo "$(LAW_DBSTR)" | sed 's/[&?]pool_max_conns=[0-9]\{1,3\}//')
export DBMATE_URL

db-up:
	dbmate --env DBMATE_URL --migrations-dir db/migrations up

db-down:
	dbmate --env DBMATE_URL --migrations-dir db/migrations down

db-status:
	dbmate --env DBMATE_URL --migrations-dir db/migrations status

db-schema:
	dbmate --env DBMATE_URL dump

# A cite string must recur this many times across the corpus to get a row in
# legalhist.stub_cases (issue #248); override with make db-stubs STUB_THRESHOLD=N.
STUB_THRESHOLD ?= 10

# Rebuild legalhist.stub_cases from the linker's misses. Uses LAW_DBSTR directly
# (needs write access). Run after cite-linker, then TRUNCATE
# moml_citations.citation_links and run cite-linker again so the citations to
# those reporters link to the stubs.
db-stubs:
	psql "$(DBMATE_URL)" -X -v ON_ERROR_STOP=1 -v threshold=$(STUB_THRESHOLD) -f db/stub_cases.sql

# Post-linker maintenance: vacuum/analyze the churned tables, then refresh all
# materialized views in dependency order (parallel within each level). Uses
# LAW_DBSTR directly (needs write access).
db-maintenance:
	./db/maintenance.sh

chambers:
	cd ./chambers && go run github.com/air-verse/air
