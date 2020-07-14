TENDERMINT_VER=0.33.2

# TODO: cleanup cayleygraph db instances, clean up partition information.

run-single-node-test:
	# need sudo becuase Docker containers run as root
	sudo rm -rf ./build/node* ./build/cayley.sock
	./build/tendermint testnet --v 1 --o ./build --populate-persistent-peers --starting-ip-address 172.17.0.2
	echo "attention: using ip address 172.17.0.2 for single node test"
	docker run -it --rm -p 26656:26656 -p 26657:26657 -v $(CURDIR)/build:/fakego/src/github.com/ANRGUSC/swarmdag/build:rw -e ID=0 anrg/swarmdag

push:
	docker push "anrg/swarmdag"

config-testnet:
	rm -rf ./build/node*
	rm -rf ./build/templates/node*
	./build/tendermint testnet --v 4 --o ./build/templates --populate-persistent-peers --starting-ip-address 192.167.10.2

update-wrapper:
	@if [ ! -d ./build ]; then mkdir build/; fi
	cp wrapper.sh ./build/wrapper.sh

get-tendermint:
	@if [ ! -d ./build ]; then mkdir build/; fi
	@if [ ! -f ./build/tendermint ]; then \
		wget https://github.com/tendermint/tendermint/releases/download/v${TENDERMINT_VER}/tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		unzip tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		rm tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		mv tendermint ./build/tendermint; \
	fi

build-docker:
	docker build -t "anrg/swarmdag" ./DOCKER/

build-cayley:
	CGO_ENABLED=0 go build -o ./build/cayley ./cmd/cayley/main.go

build: update-wrapper get-tendermint build-cayley

all: build config-testnet
	docker-compose up

clean:
	# need sudo becuase Docker containers run as root
	sudo rm -rf build/
	docker-compose down

.PHONY: build push config-testnet get-tendermint
