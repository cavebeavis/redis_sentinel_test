FROM golang:alpine AS builder 

WORKDIR /go/app

RUN apk update && apk upgrade

COPY go.mod main.go ./

RUN go mod tidy
RUN go build -o gotest main.go


FROM alpine 

RUN apk update && apk upgrade && apk add nmap

WORKDIR /var/www

COPY --from=builder /go/app/gotest .

RUN chmod +x gotest

CMD ["./gotest"]