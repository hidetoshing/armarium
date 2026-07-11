FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /armarium ./cmd/armarium

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /armarium /armarium
EXPOSE 8080
ENTRYPOINT ["/armarium"]
