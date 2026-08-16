FROM alpine:latest

ARG NAME=oida

ADD ./bin/${NAME}-linux-amd64 /usr/local/bin/oida

RUN /usr/local/bin/oida version

EXPOSE 8080

WORKDIR /app
ENTRYPOINT ["oida"]
