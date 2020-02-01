build-container:
	CGO_ENABLED=0 go build -o tm_app main.go mdns.go membership.go flags.go
	docker build . -t "anrg/test-mms"

build-test:
	CGO_ENABLED=0 go build -o tm_app main.go mdns.go membership.go flags.go

clean:
	rm tm_app

.PHONY: build push
