#!/usr/bin/python
#
# run iperf to measure the effective throughput between two nodes when
# n nodes are connected to a virtual wlan; run test for testsec
# and repeat for minnodes <= n <= maxnodes with a step size of
# nodestep
from builtins import range
import argparse
# from core import load_logging_config
from core.emulator.emudata import IpPrefixes
from core.emulator.emudata import NodeOptions

from core.emulator.enumerations import NodeTypes, EventTypes
from core.emulator.coreemu import CoreEmu
from core.location.mobility import BasicRangeModel
import random
import time

# load_logging_config()


def example(nodes):
    # ip generator for example
    prefixes = IpPrefixes("192.168.10.0/24")
    node_list = []

    # create emulator instance for creating sessions and utility methods
    coreemu = globals().get("coreemu", CoreEmu())
    session = coreemu.create_session()

    # must be in configuration state for nodes to start, when using "node_add" below
    session.set_state(EventTypes.CONFIGURATION_STATE)

    # create nodes
    node_options = NodeOptions()
    x = 0
    for i in range(nodes):
        node_options.set_position(x, 100)
        x = x+70
        session.add_node(node_options=node_options)
        node_list.append(node)
    # create switch network node
    wlan = session.add_node(_type=NodeTypes.WIRELESS_LAN)
    session.mobility.set_model(wlan, BasicRangeModel)
    for node in node_list:
        interface = prefixes.create_interface(node)
        session.add_link(node.id, wlan.id, interface_one=interface)


    # instantiate session
    session.instantiate()

if __name__ in {"__main__", "__builtin__"}:
    parser = argparse.ArgumentParser(description="Number of nodes")

    parser.add_argument("-n", "--nodes", type=int, default=8,
                        help="Number of nodes")
    options = parser.parse_args()
    example(options.nodes)
