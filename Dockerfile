# Start from golang base image
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /go/src/github.com/ONSdigital/eq-questionnaire-launcher

COPY . .

# Download dependencies
RUN go mod download

# Build the Go app
RUN echo "TARGETOS:" $TARGETOS
RUN echo "TARGETARCH" $TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -installsuffix cgo -mod mod -o /go/bin/eq-questionnaire-launcher .

######## Start a new stage from scratch #######
FROM alpine:latest

# Copy the Pre-built binary file and entry point from the previous stage
COPY --from=builder /go/bin/eq-questionnaire-launcher .
COPY docker-entrypoint.sh .
COPY static/ /static/
COPY templates/ /templates/
COPY jwt-test-keys /jwt-test-keys/

EXPOSE 8000

ENTRYPOINT ["sh", "/docker-entrypoint.sh"]
