FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY internal ./internal
COPY booking-service ./booking-service
RUN CGO_ENABLED=0 go build -o /out/booking-service ./booking-service/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/booking-service /booking-service
EXPOSE 8080
ENTRYPOINT ["/booking-service"]
