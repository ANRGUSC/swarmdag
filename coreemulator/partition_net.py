#!/usr/bin/python
#
# run iperf to measure the effective throughput between two nodes when
# n nodes are connected to a virtual wlan; run test for testsec
# and repeat for minnodes <= n <= maxnodes with a step size of
# nodestep
from builtins import range
from core.emulator.coreemu import CoreEmu
from core.emulator.data import IpPrefixes, NodeOptions
from core.emulator.enumerations import EventTypes
from core.location.mobility import BasicRangeModel
from core.nodes.base import CoreNode
from core.nodes.network import WlanNode
import time
import logging
from pyroute2 import IPRoute
import signal
import os

log = logging.getLogger()
log.setLevel(logging.INFO)

swarmdag_path = '/home/jasonatran/go/src/github.com/ANRGUSC/swarmdag/build'
num_nodes = 4

# For setting up routes on host OS
ip = IPRoute()
prefixes = IpPrefixes("192.168.10.0/24")

# create emulator instance for creating sessions and utility methods
session = CoreEmu().create_session()


def cleanup_session(signum, frame):
    print("shutting down Core nodes...")
    session.shutdown()
    if len(ip.link_lookup(ifname='veth0')) == 1:
        #  `sudo ip link del veth0 type veth peer name veth1`
        ip.link(
            "delete",
            ifname="veth0",
            kind="veth",
            peer={"ifname": "veth1"}
        )
    exit()


signal.signal(signal.SIGINT, cleanup_session)

# Docker may disable bridge net filter. Turn it back on for CORE.
os.system("echo changing this current value of " +
          "net.bridge.bridge-nf-call-iptables to 0")
os.system("cat /proc/sys/net/bridge/bridge-nf-call-iptables")
os.system("echo 0 > /proc/sys/net/bridge/bridge-nf-call-iptables")

# must be in config state for nodes to start, when using "node_add" below
session.set_state(EventTypes.CONFIGURATION_STATE)

# create nodes
node_options = NodeOptions()
node_list = []
x = 0
for i in range(num_nodes):
    x = 200
    y = 400
    # if (i - 1) % 2 == 0:  # node id starts at 1
    #     y = 400
    node_options.set_position(x, y)
    node = session.add_node(CoreNode, options=node_options)
    node_list.append(node)

# create switch network node

wlan = session.add_node(WlanNode)
session.mobility.set_model_config(
    wlan.id,
    BasicRangeModel.name,
    {
        "range": "280",
        "bandwidth": "55000000",
        "delay": "6000",
        "jitter": "5",
        "error": "5",
    },
)

for node in node_list:
    interface = prefixes.create_iface(node)
    session.add_link(node.id, wlan.id, interface)

# Delete old control bridge
try:
    ip.link("del", index=ip.link_lookup(ifname="ctrl0.1")[0])
except IndexError:
    pass # doesn't exist


session.add_remove_control_net(0, remove=False)

for n in node_list:
    session.add_remove_control_iface(
        n,
        0,
        remove=False,
        conf_required=False
    )

session.instantiate()

time.sleep(3)

for i in range(1, num_nodes + 1):
    n = session.get_node(i, CoreNode)
    print("starting swarmdag on node: %s" % n.name)
    n.cmd("route add 172.17.0.0 dev ctrl0", wait=True)
    n.cmd("route add -net 172.0.0.0 netmask 255.0.0.0 dev ctrl0", wait=True)
    n.cmd(f"sh -c '{swarmdag_path}/swarmdag > {swarmdag_path}/swarmdag{i - 1}.log'", wait=False)


# TODO: is something like this necessary for the host to talk to the CORE
# nodes? make sure to not use mask=8, this breaks ip addrs like google's which
# starts with 172!
# `route add -net 172.0.0.0 netmask 255.0.0.0 gw 172.16.0.1`
# ip.route("add", dst="172.0.0.0", mask=8, gateway="172.16.0.1")

# delete any old links
if len(ip.link_lookup(ifname='veth0')) == 1:
    # `sudo ip link del veth0 type veth peer name veth1`
    ip.link("delete", ifname="veth0", kind="veth", peer={"ifname": "veth1"})

# `sudo ip link add veth0 type veth peer name veth1`
ip.link("add", ifname="veth0", kind="veth", peer={"ifname": "veth1"})

ip.link("set", ifname="veth0", state="up")  # sudo ifconfig veth0 up
ip.link("set", ifname="veth1", state="up")  # sudo ifconfig veth1 up

# `sudo brctl addif b.ctrl0net.1 veth0`
ip.link("set", index=ip.link_lookup(ifname="veth0")[0],
        master=ip.link_lookup(ifname="ctrl0.1")[0])

# `sudo brctl addif docker0 veth1`
try:
    ip.link("set", index=ip.link_lookup(ifname="veth1")[0],
            master=ip.link_lookup(ifname="docker0")[0])
except:
    print("docker0 doesn't exist")

toggle = True
while True:
    # Note: If timeouts occur during the membership proposing process, then 20
    # sec might not be long enough of a partition time.
    time.sleep(10)
    newLoc = NodeOptions()
    if toggle is True:
        print("partitioned network")
        newLoc.set_position(1000, 1000)
        toggle = False
    else:
        print("fully connected")
        newLoc.set_position(200, 400)
        toggle = True

    session.set_node_position(session.get_node(1, CoreNode), newLoc)
    session.set_node_position(session.get_node(2, CoreNode), newLoc)
    # session.set_node_position(session.get_node(3, CoreNode), newLoc)
    # session.set_node_position(session.get_node(4, CoreNode), newLoc)
