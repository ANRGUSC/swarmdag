FROM scratch

ADD cayley /
ADD tendermint /

# Copy the code into the container
ADD . /
# COPY files/config/config.toml /conf/config.toml

#CMD ["TMHOME='/'"]
#CMD ["tendermint", "init"]

COPY node0/config/config.toml /
COPY node0/config/genesis.json /
COPY node0/config/node_key.json /
COPY node0/config/priv_validator_key.json /

# Export necessary port
EXPOSE 26656 26657

CMD ["/cayley", "-config /config.toml"]