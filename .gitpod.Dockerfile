FROM gitpod/workspace-full
RUN curl -sfL https://install.goreleaser.com/github.com/goreleaser/goreleaser.sh | sudo BINDIR=/usr/local/bin sh
