# Start from golang base image
FROM golang:1.21 AS builder

WORKDIR /go/src/github.com/ONSdigital/census31-eq-questionnaire-launcher

COPY . .

# Download dependencies
RUN go get

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -mod mod -o /go/bin/census31-eq-questionnaire-launcher .

######## Start a new stage from scratch #######
FROM alpine:latest

# Copy the Pre-built binary file and entry point from the previous stage
COPY --from=builder /go/bin/census31-eq-questionnaire-launcher .
COPY docker-entrypoint.sh .
COPY static/ /static/
COPY templates/ /templates/
COPY jwt-test-keys /jwt-test-keys/

EXPOSE 8000

ENTRYPOINT ["sh", "/docker-entrypoint.sh"]
