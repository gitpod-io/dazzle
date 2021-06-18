FROM gitpod/workspace-full:branch-rl-gitpod-add-golangci-lint-to-418
RUN curl -sfL https://install.goreleaser.com/github.com/goreleaser/goreleaser.sh | sudo BINDIR=/usr/local/bin sh
RUN sudo su -c "cd /usr; curl -L https://github.com/moby/buildkit/releases/download/v0.8.3/buildkit-v0.8.3.linux-amd64.tar.gz | tar xvz"
# NOTE: remove when workspace-full includes golangci
# RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sudo BINDIR=/usr/local/bin sh