FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod .
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/sub-adds ./cmd/server
FROM alpine:3.20
COPY --from=build /out/sub-adds /sub-adds
ENTRYPOINT ["/sub-adds"]
