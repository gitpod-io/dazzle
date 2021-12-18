FROM moby/buildkit:v0.9.3
WORKDIR /dazzle
COPY dazzle README.md /dazzle/
ENV PATH=/dazzle:$PATH
ENTRYPOINT [ "/dazzle/dazzle" ]
