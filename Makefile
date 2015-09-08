BUILD_CMD = GOOS=linux CGO_ENABLED=0 go build -installsuffix cgo -ldflags -s

.PHONY: docker

help:
	@echo "Available targets:"
	@echo ""
	@echo "  docker"

all: docker

docker:
	$(BUILD_CMD) -o kubestats
	docker build -t kubestats .
