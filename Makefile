.PHONY: help clean build-local build build-all test version release

APP_NAME := danzo
VERSION ?= dev-build
GO_OS ?= $(shell go env GOOS)
GO_ARCH ?= $(shell go env GOARCH)

CYAN := \033[0;36m
GREEN := \033[0;32m
YELLOW := \033[0;33m
NC := \033[0m

help:
	@echo "$(CYAN)$(APP_NAME) - Available targets:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

clean: ## Remove built binaries
	@rm -f $(APP_NAME) $(APP_NAME)-*
	@echo "$(GREEN)Cleaned$(NC)"

build-local: ## Build for current platform
	@go build -ldflags="-s -w -X 'github.com/tanq16/danzo/cmd.AppVersion=$(VERSION)'" -o $(APP_NAME) .
	@echo "$(GREEN)Built: ./$(APP_NAME)$(NC)"

build: ## Build for specified GO_OS/GO_ARCH
	@CGO_ENABLED=0 GOOS=$(GO_OS) GOARCH=$(GO_ARCH) go build \
		-ldflags="-s -w -X 'github.com/tanq16/danzo/cmd.AppVersion=$(VERSION)'" \
		-o $(APP_NAME)-$(GO_OS)-$(GO_ARCH) .
	@echo "$(GREEN)Built: ./$(APP_NAME)-$(GO_OS)-$(GO_ARCH)$(NC)"

build-all: ## Build all platforms
	@$(MAKE) build GO_OS=linux GO_ARCH=amd64 VERSION=$(VERSION)
	@$(MAKE) build GO_OS=linux GO_ARCH=arm64 VERSION=$(VERSION)
	@$(MAKE) build GO_OS=darwin GO_ARCH=amd64 VERSION=$(VERSION)
	@$(MAKE) build GO_OS=darwin GO_ARCH=arm64 VERSION=$(VERSION)
	@$(MAKE) build GO_OS=windows GO_ARCH=amd64 VERSION=$(VERSION)

test: ## Run tests
	@go test -v -race -cover ./...

version: ## Calculate next version
	@LATEST_TAG=$$(git tag --sort=-v:refname | head -n1 || echo "0.0.0"); \
	LATEST_TAG=$${LATEST_TAG#v}; \
	MAJOR=$$(echo "$$LATEST_TAG" | cut -d. -f1); \
	MINOR=$$(echo "$$LATEST_TAG" | cut -d. -f2); \
	PATCH=$$(echo "$$LATEST_TAG" | cut -d. -f3); \
	MAJOR=$${MAJOR:-0}; MINOR=$${MINOR:-0}; PATCH=$${PATCH:-0}; \
	COMMIT_MSG="$$(git log -1 --pretty=%B)"; \
	if echo "$$COMMIT_MSG" | grep -q "\[major-release\]"; then \
		MAJOR=$$((MAJOR + 1)); MINOR=0; PATCH=0; \
	elif echo "$$COMMIT_MSG" | grep -q "\[minor-release\]"; then \
		MINOR=$$((MINOR + 1)); PATCH=0; \
	else \
		PATCH=$$((PATCH + 1)); \
	fi; \
	echo "v$${MAJOR}.$${MINOR}.$${PATCH}"
