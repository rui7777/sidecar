FROM golang:alpine as builder

RUN apk update \
  && apk add git
WORKDIR /go/src/game-server

COPY . .
COPY . /go/src/agones.dev/agones
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server .

# final image
FROM alpine:3.11

RUN adduser -D -u 1000 server
COPY --from=builder /go/src/game-server/server /home/server/server
RUN chown -R server /home/server && \
    chmod o+x /home/server/server

USER 1000
ENTRYPOINT ["/home/server/server"]