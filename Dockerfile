FROM golang:1.27-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /social-relay ./cmd/social-relay \
    && mkdir /data

FROM scratch
COPY --from=build /social-relay /social-relay
# Owned by the runtime user so a fresh named volume inherits writable ownership.
COPY --from=build --chown=65532:65532 /data /data
USER 65532:65532
VOLUME /data
EXPOSE 2170
ENTRYPOINT ["/social-relay", "-config", "/etc/social-relay/relay.toml"]
