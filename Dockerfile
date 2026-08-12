# Start from golang base image
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /go/src/github.com/ONSdigital/census31-eq-questionnaire-launcher

COPY . .

# Download dependencies
RUN go mod download

# Build the Go app
RUN echo "TARGETOS: $TARGETOS" \
    && echo "TARGETARCH: $TARGETARCH" \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -installsuffix cgo -mod mod -o /go/bin/census31-eq-questionnaire-launcher .

######## Start a new stage from scratch #######
FROM alpine:3.20

WORKDIR /app

# Copy the Pre-built binary file and entry point from the previous stage
COPY --from=builder /go/bin/census31-eq-questionnaire-launcher .
COPY docker-entrypoint.sh .
COPY static/ /static/
COPY templates/ /templates/
COPY jwt-test-keys /jwt-test-keys/

# Create and switch to a non-root user for runtime.
RUN addgroup -S app && adduser -S -G app app \
    && chown -R app:app /app /static /templates /jwt-test-keys

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/bin/sh", "-c", "pidof census31-eq-questionnaire-launcher >/dev/null || exit 1"]

USER app

ENTRYPOINT ["sh", "/app/docker-entrypoint.sh"]
