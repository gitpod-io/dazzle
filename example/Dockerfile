FROM alpine:3.9

LABEL dazzle/layer=golang
LABEL dazzle/test=test-golang.yaml

RUN apk add --no-cache git make musl-dev go
RUN touch /dazzle-was-here
ENV PATH=/home/gitpod/.nvm/versions/node/v${NODE_VERSION}/bin:$PATH
ENV PATH=$HOME/.pyenv/bin:$HOME/.pyenv/shims:$PATH
ENV GEM_HOME=/workspace/.rvm
COPY readme.md /


LABEL dazzle/layer=node
RUN apk add --no-cache nodejs
ENV LANG=en_US.UTF-8
ENV APACHE_DOCROOT_IN_REPO="public"
ENV NGINX_DOCROOT_IN_REPO="public"
