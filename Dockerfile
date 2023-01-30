FROM moby/buildkit:v0.11.2
WORKDIR /dazzle
COPY dazzle README.md /dazzle/
ENV PATH=/dazzle:$PATH
ENTRYPOINT [ "/dazzle/dazzle" ]
