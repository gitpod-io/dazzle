FROM alpine:3.9

LABEL dazzle/layer=golang
RUN apk add --no-cache git make musl-dev go
COPY readme.md /

LABEL dazzle/layer=node
RUN apk add --no-cache nodejs
COPY readme.md /