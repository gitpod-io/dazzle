FROM docker:stable
WORKDIR /dazzle
COPY dazzle README.md /dazzle/
ENTRYPOINT [ "/dazzle/dazzle" ]