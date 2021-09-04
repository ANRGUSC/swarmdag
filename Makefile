TENDERMINT_VER=0.33.5
NUM_NODES=8

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
	./build/tendermint testnet --v ${NUM_NODES} --o ./build/templates
	go run tools/keygen.go -n ${NUM_NODES} -o ./build/templates/
.PHONY: gen-keys

get-tendermint:
	@if [ ! -d ./build ]; then mkdir build/; fi
	@if [ ! -f ./build/tendermint ]; then \
		wget https://github.com/tendermint/tendermint/releases/download/v${TENDERMINT_VER}/tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		unzip tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		rm tendermint_v${TENDERMINT_VER}_linux_amd64.zip; \
		mv tendermint ./build/tendermint; \
	fi
.PHONY: get-tendermint

build-docker:
	docker build -t "anrg/swarmdag" ./DOCKER/
.PHONY: build-docker

push:
	docker push "anrg/swarmdag"
.PHONY: push

build:
	@if [ -f ./build/swarmdag ]; then rm ./build/swarmdag; fi
	go build -o ./build/swarmdag ./cmd/swarmdag/main.go
.PHONY: build

local:
	docker-compose up
.PHONY: local

stop:
	docker-compose down
.PHONY: stop

rm_tmp:
	sudo rm -rf build/tmp/
.PHONY: rm_tmp

rm_logs:
	sudo rm -f build/tmlog*.log
.PHONY: rm_logs

run: stop build rm_tmp local
	echo "did ALL the things"
.PHONY: run

full: build rm_tmp rm_logs
	sudo core-cleanup
	tmux kill-server | true
	bash t.sh
	cd coreemulator; sudo core-python wlan_partition.py 2>&1 | tee -a ../build/main.log
.PHONY: full

clean:
	# need sudo becuase Docker containers run as root
	docker-compose down
	sudo rm -rf build/
.PHONY: clean
