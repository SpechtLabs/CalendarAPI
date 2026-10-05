# The build stage runs on the build host's platform and cross-compiles for
# the target's, so a multi-platform build needs no emulation. Keep the golang
# image in lockstep with the go pin in .mise.toml; Renovate bumps them
# together.
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine3.24 AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

WORKDIR /src
COPY src/go.mod src/go.sum ./
RUN go mod download

COPY src/ ./
# timetzdata embeds the time zone database: the runtime image has none, and
# the iCal parser loads the zones the calendars name.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags timetzdata \
    -ldflags="-s -w -X=main.version=${VERSION} -X=main.commit=${COMMIT} -X=main.date=${DATE} -X=main.builtBy=docker" \
    -o /out/calendarapi .

FROM alpine:3.24.2

LABEL org.opencontainers.image.source="https://github.com/SpechtLabs/CalendarAPI"
LABEL org.opencontainers.image.description="CalendarAPI is a service that parses iCal files and exposes their content via gRPC or a REST API."
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=build /out/calendarapi /bin/calendarapi

ENTRYPOINT ["/bin/calendarapi"]
CMD [ "serve" ]

EXPOSE     8099
EXPOSE     50051
