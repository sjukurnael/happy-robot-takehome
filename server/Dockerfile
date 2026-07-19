FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/taskman-server .

FROM alpine:3.20
COPY --from=build /bin/taskman-server /usr/local/bin/taskman-server
EXPOSE 8080
CMD ["taskman-server"]
