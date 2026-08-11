format-go:
	go fmt ./...

lint-go:
	golangci-lint run
	go run github.com/mgechev/revive@v1.15.0 -config revive.toml ./...

.PHONY: megalint megalint-apply clean-megalint
megalint:
	docker run --platform linux/amd64 --rm \
		-v /var/run/docker.sock:/var/run/docker.sock:rw \
		-v $(shell pwd):/tmp/lint:rw \
		ghcr.io/oxsecurity/megalinter:v9.6.0

megalint-apply:
	docker run --platform linux/amd64 --rm \
		-v /var/run/docker.sock:/var/run/docker.sock:rw \
		-v $(shell pwd):/tmp/lint:rw \
		-e APPLY_FIXES=all \
		ghcr.io/oxsecurity/megalinter:v9.6.0

clean-megalint:
	rm -rf megalinter-reports
