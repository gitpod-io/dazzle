ARG base
FROM ${base}

ARG GO_VERSION
ENV GO_VERSION=${GO_VERSION}

ENV GOPATH=/opt/go/go-packages
ENV GOROOT=/opt/go
ENV PATH=$GOROOT/bin:$GOPATH/bin:$PATH

RUN cd /opt; curl -fsSL https://storage.googleapis.com/golang/go$GO_VERSION.linux-amd64.tar.gz | tar xzsv