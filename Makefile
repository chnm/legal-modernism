# Cross-compilation Makefile for Go binaries
# Targets: darwin/arm64 (ARM Mac) and linux/amd64 (HPC cluster)
# Run: make -j binaries

BINARIES = adj2edge cap-import cite-detector-moml lm-diagnostic
BIN_DIR = bin

DARWIN_TARGETS = $(foreach bin,$(BINARIES),$(BIN_DIR)/darwin-arm64/$(bin))
LINUX_TARGETS = $(foreach bin,$(BINARIES),$(BIN_DIR)/linux-amd64/$(bin))

.PHONY: binaries clean test vet sync-hopper db-up db-down

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

define check_dev_env
	@if [ "$$LAW_DB_ENV" != "DEV" ]; then \
		printf "LAW_DB_ENV is not set to DEV. Are you sure? [y/N] "; \
		read ans; \
		case "$$ans" in [yY]*) ;; *) echo "Aborted."; exit 1 ;; esac; \
	fi
endef

db-up:
	$(check_dev_env)
	dbmate --url "$(DBMATE_URL)" --migrations-dir db/migrations up

db-down:
	$(check_dev_env)
	dbmate --url "$(DBMATE_URL)" --migrations-dir db/migrations down
