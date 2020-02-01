#!/usr/bin/python
#
# run iperf to measure the effective throughput between two nodes when
# n nodes are connected to a virtual wlan; run test for testsec
# and repeat for minnodes <= n <= maxnodes with a step size of
# nodestep
from builtins import range
# from core import load_logging_config
from core.emulator.emudata import IpPrefixes
from core.emulator.emudata import NodeOptions

from core.emulator.enumerations import NodeTypes, EventTypes
from core.emulator.coreemu import CoreEmu
from core.location.mobility import BasicRangeModel
import time
import logging
from pyroute2 import IPRoute

# load_logging_config()

log = logging.getLogger()

log.setLevel(logging.DEBUG)

swarmdag_path = '/home/jasonatran/go/src/github.com/tendermint/tendermint/' \
                'swarmdag/tm_app'

num_nodes = 8

# ip generator for example
prefixes = IpPrefixes("192.168.10.0/24")
node_list = []

# create emulator instance for creating sessions and utility methods
core_emu = globals().get("coreemu", CoreEmu())
session = core_emu.create_session()

# must be in config state for nodes to start, when using "node_add" below
session.set_state(EventTypes.CONFIGURATION_STATE)

# create nodes
node_options = NodeOptions()
x = 0
for i in range(num_nodes):
    x = 200
    y = 400
    # if (i - 1) % 2 == 0:  # node id starts at 1
    #     y = 400
    node_options.set_position(x, y)
    node = session.add_node(node_options=node_options)
    node_list.append(node)

# create switch network node
wlan = session.add_node(_type=NodeTypes.WIRELESS_LAN)
session.mobility.set_model(wlan, BasicRangeModel)
for node in node_list:
    interface = prefixes.create_interface(node)
    session.add_link(node.id, wlan.id, interface_one=interface)

session.add_remove_control_net(0, False, False)

for n in node_list:
    session.add_remove_control_interface(n, 0, remove=False,
                                         conf_required=False)

# instantiate session
session.instantiate()

time.sleep(5)

for n in range(1, num_nodes + 1):
    n = session.get_node(n)
    print("starting swarmdag on node: %s" % n.name)
    n.cmd(["route", "add", "172.17.0.0", "dev", "ctrl0"], wait=False)
    n.cmd(["route", "add", "-net", "172.0.0.0", "netmask", "255.0.0.0", "dev",
           "ctrl0"], wait=False)
    n.cmd(["/home/jasonatran/go/src/github.com/tendermint/tendermint/"
           "swarmdag/tm_app/tm_app"], wait=False)

# Setup routes on host OS
ip = IPRoute()

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
        master=ip.link_lookup(ifname="b.ctrl0net.1")[0])

# `sudo brctl addif docker0 veth1`
ip.link("set", index=ip.link_lookup(ifname="veth1")[0],
        master=ip.link_lookup(ifname="docker0")[0])

toggle = True
while True:
    time.sleep(15)
    newLoc = NodeOptions()
    if toggle is True:
        newLoc.set_position(1000, 1000)
        toggle = False
    else:
        newLoc.set_position(200, 400)
        toggle = True
    session.set_node_position(session.get_node(1), newLoc)
    session.set_node_position(session.get_node(2), newLoc)
    session.set_node_position(session.get_node(3), newLoc)
    session.set_node_position(session.get_node(4), newLoc)

#['/usr/local/bin/vcmd', '-c', '/tmp/pycore.1/CoreNode1', '--', 'ping', 
#'172.16.0.1']
