FROM redis:alpine

RUN apk update && apk upgrade && apk add nmap

COPY ./redis-replicas.conf /etc/redis/redis.conf

EXPOSE 6379

CMD ["redis-server" , "/etc/redis/redis.conf"]