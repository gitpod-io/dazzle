FROM gitpod/workspace-go
ENV TRIGGER_REBUILD=1
RUN curl -sfL https://install.goreleaser.com/github.com/goreleaser/goreleaser.sh | sudo BINDIR=/usr/local/bin sh
RUN sudo su -c "cd /usr; curl -L https://github.com/moby/buildkit/releases/download/v0.10.0/buildkit-v0.10.0.linux-amd64.tar.gz | tar xvz"
# NOTE: remove when workspace-full includes golangci
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sudo BINDIR=/usr/local/bin sh
