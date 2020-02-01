#!/usr/bin/env sh

##
## Input parameters
##
# BINARY=/tendermint/${BINARY:-tendermint}

# Set node_id based on assigned IP address
ID=$(($(hostname -i | cut -d "." -f 4)-2))
LOG=${LOG:-tendermint.log}

##
## Assert linux binary
##
# if ! [ -f "${BINARY}" ]; then
#     echo "The binary $(basename "${BINARY}") cannot be found. Please add the binary to the shared folder. Please use the BINARY environment variable if the name of the binary is not 'tendermint' E.g.: -e BINARY=tendermint_my_test_version"
#     exit 1
# fi
# BINARY_CHECK="$(file "$BINARY" | grep 'ELF 64-bit LSB executable, x86-64')"
# if [ -z "${BINARY_CHECK}" ]; then
#     echo "Binary needs to be OS linux, ARCH amd64"
#     exit 1
# fi

##
## Run binary with all parameters
##
export TMHOME="/tendermint/node${ID}"

# PORT=$(9000+${ID})

#start swarmdag tendermint app
# swarmdag_app -port ${PORT} &

echo "my node_id is ${ID}"

# if [ -d "`dirname ${TMHOME}/${LOG}`" ]; then
#   # --rpc.unsafe=true --log_level="*:debug"
#   tendermint "$@" --consensus.create_empty_blocks=false -- | tee "${TMHOME}/${LOG}"
# else
#   echo $ID
#   tendermint "$@" --consensus.create_empty_blocks=false
# fi

# chmod 777 -R /tendermint

/usr/bin/swarmdag_app

sleep 2147483647