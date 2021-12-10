import json
import sys
import os
import matplotlib.pyplot as plt
import signal
import numpy as np
import glob

DEFAULT_PATH = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
DEFAULT_PATH = os.path.join(DEFAULT_PATH, "build")

def signal_handler(sig, frame):
    print("Ctrl+c detected, exiting and closing all plots...")
    sys.exit(0)


class BlockPropParser:
    def __init__(self):
        self.fig, self.ax = plt.subplots()
        self.global_start_time = self.parse_global_start()
        self.transactions = {} # k: tx hash, v: list of timestamps

    def parse_global_start(self):
        pass
        # read all log files for start unixtime print and pick the lowest one.


    # sample logline:
    # unix time in ns, will be converted to ms for processing
    # `INFO:dag.go:180:{"Type": "insertTx", "Hash": "1b3d9b", "UnixTime": 1625264231}`
    def parse_file(self, file):
        with open(file, "r", errors='ignore') as f:

            for line in f:
                if line.startswith("INFO:dag.go") is False:
                    continue

                _, _, line = line.split(":", 4)

                try:
                    logline = json.loads(line)
                except json.decoder.JSONDecodeError:
                    continue

                if logline["Type"] is not "insertTx":
                    continue

                if logline["Hash"] in self.transactions:
                    self.transactions[logline["Hash"]].append(
                        int(logline["UnixTime"] / 1e6)
                    )
                else:
                    self.transactions[logline["Hash"]] = [logline["UnixTime"]]

if __name__ == '__main__':
    if len(sys.argv) == 2:
        path = sys.argv[1]
    elif len(sys.argv) == 1:
        print(f"using path {DEFAULT_PATH}")
        path = DEFAULT_PATH
    else:
        print("usage: python plot_resource.py {dir_of_logs}")
        exit()

    rPraser = BlockPropParser()

    if os.path.isdir(path) is not True:
        print("error: path not valid")
        exit()

    for f in glob.glob(path + '/swarmdag*.log'):
        print(f)

    print("Hit Ctrl-c to close figures (you may need to click on a figure)")

    # plt.tight_layout()
    # plt.show()