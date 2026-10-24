FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod .
COPY . .
RUN go mod tidy && go test ./... && CGO_ENABLED=0 go build -o /out/sub-adds ./cmd/server && CGO_ENABLED=0 go build -o /out/sub-adds-release ./cmd/client-release-ops
FROM alpine:3.20
COPY --from=build /out/sub-adds /sub-adds
COPY --from=build /out/sub-adds-release /sub-adds-release
ENTRYPOINT ["/sub-adds"]
