create-data-dir:
	mkdir -p ./data

lint:
	go vet ./...

lint-install:
	@echo "No external lint tools required; using go vet"