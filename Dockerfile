FROM alpine:3.21.2

RUN apk add --no-cache docker git

# For testing purposes
RUN apk add \
    scons \
    pkgconf \
    gcc \
    g++ \
    libx11-dev \
    libxcursor-dev \
    libxinerama-dev \
    libxi-dev \
    libxrandr-dev \
    mesa-dev \
    eudev-dev \
    alsa-lib-dev \
    pulseaudio-dev

ARG uid
ARG gid
RUN addgroup -g $gid ciserver
RUN adduser -u $uid -G ciserver -s /bin/bash ciserver -D

COPY ./server /server
ENTRYPOINT ["/server"]

USER ciserver
