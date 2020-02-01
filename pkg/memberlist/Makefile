docker-compose-memberlist:
	CGO_ENABLED=0 go build -o memberlist main.go
	docker build . -t "anrg/test-memberlist"
	docker-compose up

clean:
	rm tm_app

.PHONY: build push
