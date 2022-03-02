# Build shx in a stock Go builder container
FROM golang:alpine as builder
RUN apk add --no-cache make git gcc musl-dev linux-headers

ADD . /fastbox
ENV GO111MODULE off
RUN cd /fastbox && make fbox
# Pull shx into a second stage deploy alpine container
FROM alpine:latest
RUN echo "https://mirror.tuna.tsinghua.edu.cn/alpine/v3.4/main/" > /etc/apk/repositories

RUN apk update \
        && apk upgrade \
        && apk add --no-cache bash \
        bash-doc \
        bash-completion \
        && rm -rf /var/cache/apk/* \
        && /bin/bash
RUN apk add --no-cache ca-certificates
COPY --from=builder /fastbox/build/bin/fbox /usr/local/bin/
EXPOSE 8600 8601 27000 27000/udp
ENTRYPOINT ["fbox"]
