# Determine root directory
ROOT_DIR=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# Gather all .go files for use in dependencies below
GO_FILES=$(shell find $(ROOT_DIR) -name '*.go')

# Tool versions. NILAWAY_VERSION is the shared go-test workflow's default;
# BUF_VERSION matches the buf setup in .github/workflows. golangci-lint runs
# at latest in CI, so it is not pinned here.
BUF_VERSION=v1.71.0
GOLINES_VERSION=v0.13.0
NILAWAY_VERSION=ad240b12fb4c370017eb413f0388c71f3be8722c
GOLANGCI_LINT_MODULE=github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOVULNCHECK_VERSION=v1.7.0
GOVULNCHECK_MODULE=golang.org/x/vuln
GOVULNCHECK=go run $(GOVULNCHECK_MODULE)/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# generate and mod-tidy rewrite proto, go.mod and go.sum, so no target may run
# alongside the checks that read them, even under `make -j`.
.NOTPARALLEL:

.PHONY: all help install-tools build mod-tidy generate generate-check format format-proto golines lint lint-proto golangci-lint nilaway govulncheck vet test check

# Default target
all: format build ## Format and build (default)

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Tools land in $(go env GOBIN), or $(go env GOPATH)/bin when that is unset;
# that directory must be on PATH.
install-tools: ## Install the pinned buf, golines, and nilaway, and the latest golangci-lint
	go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	go install github.com/segmentio/golines@$(GOLINES_VERSION)
	go install go.uber.org/nilaway/cmd/nilaway@$(NILAWAY_VERSION)
	go install $(GOLANGCI_LINT_MODULE)@latest

build: mod-tidy ## Run mod-tidy, then build all Go packages
	go build ./...

mod-tidy: ## Run go mod tidy
	# Needed to fetch new dependencies and add them to go.mod
	go mod tidy

# The guard script post-processes the generated Connect code, so a bare
# `buf generate` leaves it out of date.
generate: ## Regenerate Go code from the proto files
	buf generate
	python3 scripts/guard-connect-descriptors.py
	$(MAKE) mod-tidy

generate-check: generate ## Run generate, then fail when checked-in generated code is stale
	@git diff --exit-code -- proto go.mod go.sum || { \
		echo "Generated code is out of date. Run 'make generate' and commit the result."; \
		exit 1; \
	}
	@untracked="$$(git ls-files --others --exclude-standard proto)"; \
	if [ -n "$$untracked" ]; then \
		echo "$$untracked"; \
		echo "Generated code is out of date. Run 'make generate' and commit the result."; \
		exit 1; \
	fi

format: mod-tidy format-proto ## Run mod-tidy, then format proto and Go code
	go fmt ./...
	gofmt -s -w $(GO_FILES)

format-proto: ## Format proto files
	buf format -w

golines: ## Enforce 80-character line limit
	golines -w --ignore-generated --chain-split-dots --max-len=80 --reformat-tags .

lint: lint-proto golangci-lint nilaway ## Run buf, golangci-lint, and nilaway

lint-proto: ## Fail on unformatted or non-conforming proto files
	buf format -d --exit-code
	buf lint

golangci-lint: ## Run golangci-lint
	golangci-lint run ./...

nilaway: ## Fail on NilAway findings
	nilaway ./...

govulncheck: ## Fail on known vulnerabilities reachable from source
	GOTOOLCHAIN=auto $(GOVULNCHECK) ./...

vet: ## Run go vet
	go vet ./...

test: mod-tidy ## Run mod-tidy, then all tests with race detection
	go test -v -race ./...

check: generate-check lint vet build test govulncheck ## Run the checks CI runs
