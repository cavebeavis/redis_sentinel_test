FROM redis:alpine

RUN apk update && apk upgrade && apk add nmap

EXPOSE 6379

CMD ["redis-server"]