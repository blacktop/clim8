.PHONY: bump
bump:
	@echo "🚀 Bumping Version"
	git tag -a $(shell svu patch) -m "Release $(shell svu patch)"
	git push --tags

.PHONY: build
build:
	@echo "🚀 Building Version $(shell svu current)"
	go build -o clim8 main.go

.PHONY: release
release:
	@echo "🚀 Releasing Version $(shell svu current)"
	goreleaser build --id default --clean --snapshot --single-target --output dist/clim8

.PHONY: snapshot
snapshot:
	@echo "🚀 Snapshot Version $(shell svu current)"
	goreleaser --clean --timeout 60m --snapshot
