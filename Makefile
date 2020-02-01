build-swarmdag:
	@if [ -f swarmdag_app ]; then rm swarmdag_app; fi
	CGO_ENABLED=0 go build -o swarmdag_app tm_app/main.go 
	docker build -t "anrg/swarmdag" .

run-single-node-test:
	sudo rm -rf build/*
	tendermint testnet --v 1 --o ./build --populate-persistent-peers --starting-ip-address 172.17.0.2
	docker run -it --rm -p 26656 -p 26657 -v $(CURDIR)/build:/tendermint:rw -e ID=0 anrg/swarmdag

push:
	docker push "anrg/swarmdag"

build-tendermint:
	@if [ -f tendermint ]; then rm tendermint; fi
	cd ..; make build; mv build/tendermint swarmdag/tendermint

reset-testnet:
	sudo rm -rf build/*
	tendermint testnet --v 4 --o ./build --populate-persistent-peers --starting-ip-address 172.17.0.2

clean:
	sudo rm -rf build/*
	rm swarmdag_app

.PHONY: build push
