FROM golang:1.25.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o db-schema-sync cmd/main.go

FROM alpine:latest

# Install curl and postgresql-client for psqldef
RUN apk add --no-cache curl postgresql-client

# Download and install psqldef
RUN curl -LO https://github.com/sqldef/sqldef/releases/download/v3.9.4/psqldef_linux_amd64.tar.gz \
    && tar xzf psqldef_linux_amd64.tar.gz \
    && mv psqldef /usr/local/bin/ \
    && rm psqldef_linux_amd64.tar.gz

# Copy the binary from builder stage
COPY --from=builder /app/db-schema-sync /usr/local/bin/db-schema-sync

ENTRYPOINT ["db-schema-sync"]