#!/usr/bin/env sh
# Set node_id based on assigned IP address

ID=$(($(hostname -i | cut -d "." -f 4)-2))

#export TMHOME="/tendermint/node${ID}"
#export TMHOME="/home/loesing/TMHOME/node${ID}"
export TMHOME="/TMHOME/node${ID}"

#PORT=$(9000+${ID})

#start cayley ABCI app

# Current cayley application does not take a port anymore
#cayley -port ${PORT} &
chmod 777 -R /usr/bin/cayley
/usr/bin/cayley &

echo "my node_id is ${ID}"

chmod 777 -R /usr/bin/tendermint

#/tendermint --consensus.create_empty_blocks=false 
/usr/bin/tendermint node --proxy_app=unix://cayley.sock

sleep 2147483647