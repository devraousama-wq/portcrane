FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /portcrane ./cmd/portcrane

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /portcrane /usr/local/bin/portcrane
EXPOSE 8080 8443 9090
ENTRYPOINT ["/usr/local/bin/portcrane"]
