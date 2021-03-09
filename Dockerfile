FROM docker:stable
WORKDIR /dazzle
COPY dazzle README.md /dazzle/
ENV PATH=/dazzle:$PATH
ENTRYPOINT [ "/dazzle/dazzle" ]