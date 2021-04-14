NAME=scaffolding
BINARY=packer-plugin-${NAME}

COUNT?=1
TEST?=$(shell go list ./...)

.PHONY: dev

build:
	@go build -o ${BINARY}

dev: build
	@mkdir -p ~/.packer.d/plugins/
	@mv ${BINARY} ~/.packer.d/plugins/${BINARY}

run-example: dev
	@packer build ./example

test:
	@go test -count $(COUNT) $(TEST) -timeout=3m

testacc: dev
	@PACKER_ACC=1 go test -count $(COUNT) -v $(TEST) -timeout=120m