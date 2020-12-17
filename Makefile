TENDERMINT_VER=0.33.5

# TODO: cleanup cayleygraph db instances, clean up partition information.

# run-single-node-test:
# 	# need sudo becuase Docker containers run as root
# 	sudo rm -rf ./build/node* ./build/cayley.sock
# 	./build/tendermint testnet --v 1 --o ./build --populate-persistent-peers --starting-ip-address 172.17.0.2
# 	echo "attention: using ip address 172.17.0.2 for single node test"
# 	docker run -it --rm -p 26656:26656 -p 26657:26657 -v $(CURDIR)/build:/fakego/src/github.com/ANRGUSC/swarmdag/build:rw -e ID=0 anrg/swarmdag

gen-keys:
	@if [ -d .build/templates ]; then rm -rf ./build/templates/node*; fi
	@if [ ! -d ./build/templates/ ]; then mkdir -p ./build/templates; fi
	./build/tendermint testnet --v 4 --o ./build/templates
	go run tools/keygen.go -n 4 -o ./build/templates/

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

push:
	docker push "anrg/swarmdag"

build:
	CGO_ENABLED=0 go build -o ./build/swarmdag ./cmd/swarmdag/main.go

local:
	docker-compose up

stop:
	docker-compose down

rm_tmp:
	sudo rm -rf build/tmp/

clean:
	# need sudo becuase Docker containers run as root
	docker-compose down
	sudo rm -rf build/

.PHONY: gen-keys get-tendermint build build-docker push build-partition local stop clean rm_tmp
