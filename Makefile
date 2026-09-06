# twaictl 開發用 Makefile。所有 build-time 工具都以 go run 固定版本呼叫，不進 go.mod。
OAPI_CODEGEN_VERSION ?= v2.8.0
GORELEASER_VERSION   ?= v2.18.0
GOLANGCI_LINT        ?= golangci-lint

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X github.com/gilbertchiao/twaictl/internal/version.Version=$(VERSION) \
  -X github.com/gilbertchiao/twaictl/internal/version.Commit=$(COMMIT) \
  -X github.com/gilbertchiao/twaictl/internal/version.Date=$(DATE)

.PHONY: build test test-integration lint fmt tidy gen gen-check docs docs-check snapshot clean

build: ## 編譯到 ./bin/twaictl
	go build -ldflags '$(LDFLAGS)' -o bin/twaictl ./cmd/twaictl

test: ## 執行所有測試
	go test ./... -race -count=1

test-integration: ## 對真實 API 做 read-only 驗證；需要 work/api-key.md（不進版控）
	@test -f work/api-key.md || { echo "缺少 work/api-key.md"; exit 1; }
	@TWAI_API_KEY="$$(sed -n 's/^api-key: *//p' work/api-key.md)" \
	 TWAI_PROJECT_CODE="$$(sed -n 's/^project: *//p' work/api-key.md)" \
	 go test -tags integration ./internal/twai/ -run Integration -v -count=1

lint: ## 靜態檢查
	$(GOLANGCI_LINT) run ./...

fmt: ## 格式化
	gofmt -s -w $$(git ls-files '*.go' | grep -v '^internal/gen/')

tidy:
	go mod tidy

gen: ## 由 api/openapi/*.yaml 重新生成 internal/gen/ 與 OpenAPI 版本常數
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -config api/oapi-codegen.vcs.yaml    api/openapi/VCS.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -config api/oapi-codegen.ceph.yaml   api/openapi/Ceph.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -config api/oapi-codegen.common.yaml api/openapi/Common.yaml
	sh scripts/gen-openapi-versions.sh > internal/version/openapi.go
	gofmt -w internal/version/openapi.go

gen-check: gen ## CI 用：確認生成產物沒有漂移
	git diff --exit-code -- internal/gen internal/version/openapi.go

docs: ## 由 cobra 命令樹重新生成 docs/commands.md
	go run ./tools/gendocs > docs/commands.md.tmp
	mv docs/commands.md.tmp docs/commands.md

docs-check: docs ## CI 用：確認 docs/commands.md 沒有漂移（提示執行 make docs 重新生成）
	git diff --exit-code -- docs/commands.md

snapshot: ## 用 goreleaser 產出本機 snapshot binary（dist/）
	go run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION) release --snapshot --clean

clean:
	rm -rf bin dist
