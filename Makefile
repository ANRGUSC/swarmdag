TENDERMINT_VER=0.33.2

build-swarmdag:
	@if [ -f swarmdag_app ]; then rm swarmdag_app; fi
	CGO_ENABLED=0 go build -o swarmdag_app tm_app/main.go 
	docker build -t "anrg/swarmdag" .

run-single-node-test:
	sudo rm -rf build/node*
	tendermint testnet --v 1 --o ./build --populate-persistent-peers --starting-ip-address 172.17.0.2
	docker run -it --rm -p 26656 -p 26657 -v $(CURDIR)/build:/tendermint:rw -e ID=0 anrg/swarmdag

push:
	docker push "anrg/swarmdag"

get-tendermint:
	@if [ ! -d ./build ]; then mkdir build/; fi
	@if [ ! -f ./build/tendermint ]; then \
		wget https://github.com/tendermint/tendermint/releases/download/v${TENDERMINT_VER}/tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		unzip tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		rm tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		mv tendermint ./build/tendermint; \
	fi

reset-testnet:
	sudo rm -rf build/*
	tendermint testnet --v 4 --o ./build --populate-persistent-peers --starting-ip-address 172.17.0.2

clean:
	rm -rf build/*

.PHONY: build push
