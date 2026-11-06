FROM golang:1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /hub ./cmd/hub

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /hub /hub
ENTRYPOINT ["/hub", "-config", "/etc/jomaa-home-hub/config.json"]
