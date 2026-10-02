FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o /catalog-service .

FROM alpine:3.22
COPY --from=build /catalog-service /catalog-service
EXPOSE 8000
CMD ["/catalog-service"]
