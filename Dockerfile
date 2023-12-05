FROM --platform=$BUILDPLATFORM golang:1.21.4 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -installsuffix cgo -o main .

FROM --platform=$BUILDPLATFORM golang:1.21.4
WORKDIR /
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
