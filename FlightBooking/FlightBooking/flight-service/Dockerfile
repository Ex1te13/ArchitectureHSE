FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY internal ./internal
COPY flight-service ./flight-service
RUN CGO_ENABLED=0 go build -o /out/flight-service ./flight-service/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/flight-service /flight-service
EXPOSE 50051
ENTRYPOINT ["/flight-service"]
