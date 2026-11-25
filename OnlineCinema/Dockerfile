FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum ./

FROM build AS health-build
COPY cmd/healthcheck ./cmd/healthcheck
RUN CGO_ENABLED=0 go build -o /out/healthcheck ./cmd/healthcheck

FROM build AS schema-init-build
COPY cmd/schema-init ./cmd/schema-init
RUN CGO_ENABLED=0 go build -o /out/schema-init ./cmd/schema-init

FROM scratch AS schema-init
WORKDIR /app
COPY --from=schema-init-build /out/schema-init /schema-init
COPY schemas/movie-event.avsc ./schemas/movie-event.avsc
ENTRYPOINT ["/schema-init"]

FROM build AS producer-build
RUN go mod download
COPY cmd/producer ./cmd/producer
COPY internal/producer ./internal/producer
RUN CGO_ENABLED=0 go build -o /out/producer ./cmd/producer

FROM scratch AS producer
COPY --from=health-build /out/healthcheck /healthcheck
COPY --from=producer-build /out/producer /producer
EXPOSE 8080
ENTRYPOINT ["/producer"]

FROM build AS pipeline-test-build
COPY integration ./integration
RUN CGO_ENABLED=0 go test -c -o /out/pipeline-test ./integration

FROM scratch AS pipeline-test
COPY --from=pipeline-test-build /out/pipeline-test /pipeline-test
ENTRYPOINT ["/pipeline-test", "-test.v", "-test.timeout=90s"]

FROM build AS aggregation-build
RUN go mod download
COPY cmd/aggregation ./cmd/aggregation
COPY internal/aggregation ./internal/aggregation
COPY internal/export ./internal/export
RUN CGO_ENABLED=0 go build -o /out/aggregation ./cmd/aggregation

FROM scratch AS aggregation
COPY --from=health-build /out/healthcheck /healthcheck
COPY --from=aggregation-build /out/aggregation /aggregation
EXPOSE 8082
ENTRYPOINT ["/aggregation"]
