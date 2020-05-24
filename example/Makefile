DAZZLE=dazzle
CACHE_REF=hub.docker.io/some-repo
DEST_REF=hub.docker.io/some-other-repo
BUILDKIT_ADDR=unix:///run/buildkit/buildkitd.sock

.PHONY: chunks combinations

chunks:
	${DAZZLE} build --addr ${BUILDKIT_ADDR} --no-cache ${CACHE_REF} .

combinations:
	${DAZZLE} combine --addr ${BUILDKIT_ADDR} ${DEST_REF}:all ${CACHE_REF} chunk1 chunk2

