
ARG base
FROM ${base}

ARG NODE_VERSION
ARG NVM_VERSION=0.37.2

RUN curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v${NVM_VERSION}/install.sh | PROFILE=/dev/null bash
RUN bash -c "source $HOME/.nvm/nvm.sh \
        && nvm install $NODE_VERSION \
        && nvm alias default $NODE_VERSION"
# above, we are adding the lazy nvm init to .bashrc, because one is executed on interactive shells, the other for non-interactive shells (e.g. plugin-host)
COPY nvm-lazy.sh /root/.nvm/nvm-lazy.sh
ENV PATH=$PATH:/root/.nvm/versions/node/v${NODE_VERSION}/bin