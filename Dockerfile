FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG CMD_PATH=./cmd/api
RUN CGO_ENABLED=0 go build -o /flashcart-bin ${CMD_PATH}

FROM alpine:3.20
COPY --from=build /flashcart-bin /flashcart-bin
EXPOSE 8080
ENTRYPOINT ["/flashcart-bin"]
