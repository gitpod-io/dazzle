FROM gitpod/workspace-full

USER root
ENV TRIGGER_REBUILD=2
RUN echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list \
    && install-packages goreleaser -y
RUN sudo su -c "cd /usr; curl -L https://github.com/moby/buildkit/releases/download/v0.11.2/buildkit-v0.11.2.linux-amd64.tar.gz | tar xvz"
# NOTE: remove when workspace-full includes golangci
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sudo BINDIR=/usr/local/bin sh

USER gitpod