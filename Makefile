# Makefile for Go project

default: build

# All target
all: ui
	$(MAKE) build

ui: ui-admin ui-user

ui-admin:
	cd ui/admin && \
	pnpm install

ui-user:
	cd ui/user && \
	pnpm install && \
	pnpm run build

clean:
	rm -rf ui/admin/build
	rm -rf ui/user/build

serve-docs:
	cd docs && \
	npm install && \
	npm run start

# Build the project
build:
	go build -ldflags="-s -w" -o bin/otto8 .


dev:
	./tools/dev.sh $(ARGS)

dev-open: ARGS=--open-uis
dev-open: dev

# Lint the project
lint: lint-admin

lint-admin:
	cd ui/admin && \
	pnpm run format && \
	pnpm run lint

package-tools:
	./tools/package-tools.sh

no-changes:
	@if [ -n "$$(git status --porcelain)" ]; then \
		git status --porcelain; \
		git --no-pager diff; \
		echo "Encountered dirty repo!"; \
		exit 1; \
	fi

.PHONY: ui ui-admin ui-user build all clean dev dev-open lint lint-admin lint-api no-changes fmt tidy
