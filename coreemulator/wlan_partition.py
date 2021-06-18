#!/usr/bin/python
from builtins import range
from typing import List
from core.emulator.coreemu import CoreEmu
from core.emulator.data import IpPrefixes, NodeOptions, InterfaceData
from core.emulator.enumerations import EventTypes
from core.location.mobility import BasicRangeModel
from core.nodes.base import CoreNode
from core.nodes.network import WlanNode
import time
import logging
from pyroute2 import IPRoute
import signal
import os
import net_events

log = logging.getLogger()
log.setLevel(logging.INFO)

SWARMDAG_PATH = '/home/jasonatran/go/src/github.com/ANRGUSC/swarmdag/build'
NUM_NODES = 8
prefixes = IpPrefixes("192.168.10.0/24")
session = CoreEmu().create_session()
ip = IPRoute()  # for IP routes on host


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


# Link or unlink nodes in both directions
def wlan_link(node1, node2, wlan, unlink=False):
    pairs = {node1: node2, node2: node1}
    for src, dst in pairs.items():
        node1_iface, node2_iface = None, None
        for net, iface1, iface2 in src.commonnets(dst):
            if net == wlan:
                node1_iface = iface1
                node2_iface = iface2
                break
        if node1_iface and node2_iface:
            if unlink:
                wlan.unlink(node1_iface, node2_iface)
            else:
                wlan.link(node1_iface, node2_iface)


def create_wlan(session, nodes, ifaces) -> WlanNode:
    wlan = session.add_node(WlanNode, options=NodeOptions(x=100, y=100))
    session.mobility.set_model_config(
        wlan.id,
        BasicRangeModel.name,
        {
            "range": "280",
            "bandwidth": "55000000",
            # "delay": "6000",
            # "jitter": "5",
            # "error": "5",
        },
    )
    for n, i in zip(nodes, ifaces):
        session.add_link(n.id, wlan.id, i)
    return wlan


def create_nodes(session, num_nodes) -> (List[CoreNode], List[InterfaceData]):
    node_options = NodeOptions(x=100, y=100)
    nodes, ifaces = [], []
    for i in range(num_nodes):
        n = session.add_node(CoreNode, options=node_options)
        iface = prefixes.create_iface(n)
        nodes.append(n)
        ifaces.append(iface)
    return nodes, ifaces


def add_ctrl_bridge(session, nodes):
    # Delete any existing control bridge
    try:
        ip.link("del", index=ip.link_lookup(ifname="ctrl0.1")[0])
    except IndexError:
        pass  # does not exist

    session.add_remove_control_net(0, remove=False)
    for n in nodes:
        session.add_remove_control_iface(
            n,
            0,
            remove=False,
            conf_required=False
        )
        n.cmd("route add 172.17.0.0 dev ctrl0", wait=True)
        n.cmd(
            "route add -net 172.0.0.0 netmask 255.0.0.0 dev ctrl0",
            wait=True
        )


def exec_swarmdag(session, nodes):
    for idx, n in enumerate(nodes):
        print("starting swarmdag on node: %s" % n.name)
        n.cmd(
            f"sh -c '{SWARMDAG_PATH}/swarmdag >> {SWARMDAG_PATH}/swarmdag{idx}.log 2>&1'",
            wait=False
        )


# Briges the control net and docker net so containers between the two networks
# can talk to each other
def bridge_ctrlnet_and_docker():
    # Adding this route may be needed to successfully connect. Uncomment as
    # needed. Make sure to not use mask=8 as this breaks ip addrs like google's
    # which starts with `172.`.
    # `route add -net 172.0.0.0 netmask 255.0.0.0 gw 172.16.0.1`
    # ip.route("add", dst="172.0.0.0", mask=(?), gateway="172.16.0.1")

    # delete any old links
    if len(ip.link_lookup(ifname='veth0')) == 1:
        # `sudo ip link del veth0 type veth peer name veth1`
        ip.link(
            "delete",
            ifname="veth0",
            kind="veth",
            peer={"ifname": "veth1"}
        )

    # `sudo ip link add veth0 type veth peer name veth1`
    ip.link("add", ifname="veth0", kind="veth", peer={"ifname": "veth1"})
    ip.link("set", ifname="veth0", state="up")  # sudo ifconfig veth0 up
    ip.link("set", ifname="veth1", state="up")  # sudo ifconfig veth1 up

    # `sudo brctl addif b.ctrl0net.1 veth0`
    ip.link("set", index=ip.link_lookup(ifname="veth0")[0],
            master=ip.link_lookup(ifname="ctrl0.1")[0])

    # `sudo brctl addif docker0 veth1`
    try:
        ip.link(
            "set",
            index=ip.link_lookup(ifname="veth1")[0],
            master=ip.link_lookup(ifname="docker0")[0]
        )
    except IndexError:
        print("docker0 iface doesn't exist")


signal.signal(signal.SIGINT, cleanup_session)

# Docker may disable bridge net filter. Turn it back on for CORE.
os.system("echo 0 > /proc/sys/net/bridge/bridge-nf-call-iptables")
print("Warning: disabling bridge-nf-call-iptables. This may break Docker?")

# must be in config state for nodes to start, when using "node_add" below
session.set_state(EventTypes.CONFIGURATION_STATE)
nodes, ifaces = create_nodes(session, NUM_NODES)
wlan = create_wlan(session, nodes, ifaces)
session.instantiate()

watch_dir = os.path.join(SWARMDAG_PATH, "partition_done")

try:
    os.mkdir(watch_dir)
except OSError as error:
    print(error)

for f in os.listdir(watch_dir):
    print(f)
    print(watch_dir)


evt_gen = net_events.Generator(
    split_rate=1 / 15,
    merge_rate=1 / 10,
    topology="fully_connected",
    nodes=nodes,
    wlan=wlan,
    watch_dir=watch_dir,
    log=log
)

exec_swarmdag(session, nodes)
evt_gen.run()
