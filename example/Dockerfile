FROM alpine:3.9

LABEL dazzle/layer=golang
RUN apk add --no-cache git make musl-dev go
RUN touch /dazzle-was-here
ENV PATH=/home/gitpod/.nvm/versions/node/v${NODE_VERSION}/bin:$PATH
ENV PATH=$HOME/.pyenv/bin:$HOME/.pyenv/shims:$PATH
ENV GEM_HOME=/workspace/.rvm
COPY readme.md /

LABEL dazzle/layer=node
RUN apk add --no-cache nodejs
ENV LANG=en_US.UTF-8
ENV HOME=/home/gitpod
ENV APACHE_DOCROOT_IN_REPO="public"
ENV NGINX_DOCROOT_IN_REPO="public"
ENV PATH="$PATH:/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin/"
ENV MANPATH="$MANPATH:/home/linuxbrew/.linuxbrew/share/man"
ENV INFOPATH="$INFOPATH:/home/linuxbrew/.linuxbrew/share/info"
ENV GO_VERSION=1.12 \
    GOPATH=$HOME/go-packages \
    GOROOT=$HOME/go
ENV PATH=$GOROOT/bin:$GOPATH/bin:$PATH
ENV GOPATH=/workspace/go \
    PATH=/workspace/go/bin:$PATH
ENV GRADLE_USER_HOME=/workspace/.gradle/