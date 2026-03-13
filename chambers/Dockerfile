FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /chambers ./chambers/

FROM gcr.io/distroless/static-debian12
COPY --from=builder /chambers /chambers
EXPOSE 4567
ENTRYPOINT ["/chambers"]
