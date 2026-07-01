lint: lint-go lint-templates lint-static

format: format-go format-static format-templates

format-go:
	go fmt ./...

lint-go:
	golangci-lint run

lint-static:
	npx prettier --check "static/**/*.{js,css}"
	npx eslint static/javascript/*.js

format-static:
	npx prettier "static/**/*.{js,css}" --write

lint-templates:
	poetry run djlint templates --lint

format-templates:
	poetry run djlint templates/*.html --reformat

.PHONY: megalint megalint-apply clean-megalint
megalint:
	docker run --platform linux/amd64 --rm \
		-v /var/run/docker.sock:/var/run/docker.sock:rw \
		-v $(shell pwd):/tmp/lint:rw \
		ghcr.io/oxsecurity/megalinter:v9.5.0

megalint-apply:
	docker run --platform linux/amd64 --rm \
		-v /var/run/docker.sock:/var/run/docker.sock:rw \
		-v $(shell pwd):/tmp/lint:rw \
		-e APPLY_FIXES=all \
		ghcr.io/oxsecurity/megalinter:v9.5.0

clean-megalint:
	rm -rf megalinter-reports
