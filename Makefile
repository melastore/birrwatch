DB      ?= birrwatch.db
HISTORY ?= data/rates.csv
ADDR    ?= :8080

.PHONY: help test build scrape serve web dev fmt check clean

help:
	@echo "make test    - run Go tests and the web typecheck"
	@echo "make build   - build both binaries into ./bin and the dashboard into web/dist"
	@echo "make scrape  - fetch today's rates from NBE and the parallel CSV"
	@echo "make serve   - run the API and dashboard on $(ADDR)"
	@echo "make dev     - run the Vite dev server (expects 'make serve' in another shell)"
	@echo "make check   - gofmt, vet and tests; what CI runs"

test:
	cd server && go test ./...
	cd web && npm run typecheck

build:
	cd server && go build -o ../bin/birrd ./cmd/birrd
	cd server && go build -o ../bin/birrscrape ./cmd/birrscrape
	cd web && npm run build

scrape: build
	./bin/birrscrape -db $(DB) -import $(HISTORY)
	./bin/birrscrape -db $(DB) -source nbe
	./bin/birrscrape -db $(DB) -source parallel -parallel-csv data/parallel.csv -export $(HISTORY)

serve: build
	./bin/birrd -db $(DB) -addr $(ADDR) -web web/dist

dev:
	cd web && npm run dev

fmt:
	cd server && gofmt -w ./cmd ./internal

check:
	cd server && test -z "$$(gofmt -l ./cmd ./internal)" || (echo "run 'make fmt'"; exit 1)
	cd server && go vet ./...
	cd server && go test ./...
	cd web && npm run build

clean:
	rm -rf bin web/dist $(DB) $(DB)-wal $(DB)-shm
