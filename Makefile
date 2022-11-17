REVERSION ?= 2
VERSION ?= 0.0.${REVERSION}


default: linux par

linux:
	GOARCH=amd64 GOOS=linux CGO_ENABLED=0 go build -o bin/http-provider

native:
	CGO_ENABLED=0 go build -o bin/http-provider

mac-par:
	mkdir -p dist
	wash par create --arch x86_64-macos   --binary bin/http-provider         --capid wasmcloud:httpserver --name "http-provider" --vendor taction --version $(VERSION) --revision $(REVERSION) --destination dist/httpprovider.par.gz --compress
par:
	mkdir -p dist
	wash par create --arch x86_64-linux   --binary bin/http-provider         --capid wasmcloud:httpserver --name "http-provider" --vendor taction --version $(VERSION) --revision $(REVERSION) --destination dist/httpprovider.par.gz --compress
	#wash par insert --arch x86_64-linux   --binary bin/weekv-provider         dist/1_litestream.par.gz
push:
	wash reg push index.docker.io/docker4zc/dapr-provider-go:$(VERSION) dist/httpprovider.par.gz

push-mac:
	wash reg push index.docker.io/docker4zc/dapr-provider-go-mac:$(VERSION) dist/httpprovider.par.gz

clean:
	rm -rf bin dist

bp: clean linux par push

np: clean native mac-par push-mac