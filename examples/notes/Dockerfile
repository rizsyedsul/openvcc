# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/notes .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/notes /notes
EXPOSE 8000
USER nonroot:nonroot
ENTRYPOINT ["/notes"]
