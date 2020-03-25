#!/usr/bin/env sh

# Set node_id based on assigned IP address
ID=$(($(hostname -i | cut -d "." -f 4)-2))
LOG=node${ID}-tendermint.log

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

export SWARMDAG_BUILD_PATH="$GOPATH/src/github.com/ANRGUSC/swarmdag/build"
export TMHOME="${SWARMDAG_BUILD_PATH}/node${ID}"

# PORT=$(9000+${ID})

echo "my node_id is ${ID}"

# if [ -d "`dirname ${TMHOME}/${LOG}`" ]; then
#   # --rpc.unsafe=true --log_level="*:debug"
#   tendermint "$@" --consensus.create_empty_blocks=false -- | tee "${TMHOME}/${LOG}"
# else
#   echo $ID
#   tendermint "$@" --consensus.create_empty_blocks=false
# fi

chmod 777 -R ${SWARMDAG_BUILD_PATH}

${SWARMDAG_BUILD_PATH}/cayley &
${SWARMDAG_BUILD_PATH}/tendermint node --rpc.laddr=tcp://0.0.0.0:26657 \
    --consensus.create_empty_blocks=false -- | tee "${TMHOME}/${LOG}"

# sleep 2147483647