FROM golang:1.26-bookworm AS build

WORKDIR /src
ENV GOWORK=off
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /timmyd ./cmd/timmyd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /timmyd /timmyd
ENTRYPOINT ["/timmyd"]
