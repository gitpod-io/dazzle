FROM alpine:3.9
WORKDIR /dazzle
COPY dazzle README.md /dazzle/
ENTRYPOINT [ "/dazzle/dazzle" ]