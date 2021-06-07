DAZZLE=sudo ./dazzle
CACHE_REF=localhost:5000/example-cache
DEST_REF=localhost:5000/example
BUILDKIT_ADDR=unix:///run/buildkit/buildkitd.sock

.PHONY: build chunks combinations

chunks: build
	${DAZZLE} build --addr ${BUILDKIT_ADDR} --no-cache ${CACHE_REF} .

combinations: build 
	${DAZZLE} combine --addr ${BUILDKIT_ADDR} ${DEST_REF}:all ${CACHE_REF} chunk1 chunk2

# hacky, but keeping it self-contained
build: 
	go build ../

gen: 
	go generate ../... 

clean: 
	rm -f ./dazzle
	# Heavy handed
	(docker stop registry || true) && docker run --rm -d -p 5000:5000 --name registry registry:2