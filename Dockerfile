FROM moby/buildkit:v0.10.1
WORKDIR /dazzle
COPY dazzle README.md /dazzle/
ENV PATH=/dazzle:$PATH
ENTRYPOINT [ "/dazzle/dazzle" ]
