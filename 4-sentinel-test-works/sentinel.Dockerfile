FROM redis:alpine

RUN apk update && apk upgrade && apk add nmap

COPY ./sentinel.conf /etc/sentinel.conf

RUN chmod a+w /etc/sentinel.conf

RUN echo -e '#!/bin/sh \nset -e \n/usr/local/bin/redis-sentinel /etc/sentinel.conf \nexec "$@"' > /usr/local/bin/docker-entrypoint.sh

EXPOSE 26379