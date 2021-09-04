import json
import sys
import os
import matplotlib.pyplot as plt
import signal
import numpy as np


def signal_handler(sig, frame):
    print("Ctrl+c detected, exiting and closing all plots...")
    sys.exit(0)

class BlockPropParser:
    def __init__(self):


    # sample logline:  `INFO:dag.go:180:{"type": "insertTx", "hash": "1b3d9b", "unixTime": 1625264231}`
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

                if logline["type"] is "insertTx":

                    hash = logline["Values"]["hash"]
                    if hash not in self.blocks:
                        self.blocks[hash] = {
                            "mined": 0,
                            "importTimes": []
                        }
                    if logline["message"].startswith("Imported new chain segment"):
                        self.blocks[hash]["importTimes"].append(
                            int(logline["unixNanoTime"] / 1e6)
                        )
                        self.total_final_blocks = max(logline["Values"]["number"],
                                                      self.total_final_blocks)
                    if logline["message"].startswith("Successfully sealed new"):
                        self.blocks[hash]["mined"] = logline["unixNanoTime"] / 1e6
                    if logline["message"].startswith("Chain reorg detected"):
                        # number of blocks added from reorg
                        # (logs don't indicate how many blocks dropped in reorg)
                        # reorg_cnt += int(logline["Values"]["add"])
                        reorg_cnt += 1

            self.reorgs[file] = reorg_cnt
            self.total_reorgs += reorg_cnt

if __name__ == '__main__':
    if len(sys.argv) == 2:
        path = sys.argv[1]
    else:
        print("usage: python plot_resource.py {file_or_dir}")
        exit()

    rPraser = ResourceParser()

    if os.path.isdir(path):
        files = os.listdir(path)
        os.chdir(path)
    else:
        files = [path]

    cnt = 0
    for f in files:
        if "-tx-" not in f:
            resources = rPraser.parse_file(f)
            rPraser.plot_resources(resources)
            cnt += 1
            if cnt == MAX_FILES:
                break

    print("Hit Ctrl-c to close figures (you may need to click on a figure)")

    plt.tight_layout()
    plt.show()
