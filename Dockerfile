# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/openvcc ./cmd/openvcc

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/openvcc /openvcc
EXPOSE 8080 9090 8081
USER nonroot:nonroot
ENTRYPOINT ["/openvcc"]
