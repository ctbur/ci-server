FROM alpine:3.21.2

RUN apk add --no-cache docker git

COPY ./server /server
ENTRYPOINT ["/server"]

ARG user
USER $user
