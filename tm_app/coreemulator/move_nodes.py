import subprocess
import argparse
import time
coresendmsgpath = "/home/jasonatran/core/daemon/scripts/coresendmsg"

#Parsing args from cmd line
parser = argparse.ArgumentParser(description="move nodes arguments")
parser.add_argument("-g", "--group", type=int, default=1,
                    help="Choosing which group to move")
parser.add_argument("-n", "--nodes", type=int, default=8,
                    help="Number of nodes")
parser.add_argument("-s", "--sizeg1", type=int, default=4,
                    help="Size of group 1")
parser.add_argument("-r", "--recombine", type=bool, default=False,
                    help="Recombine previously split nodes")
options = parser.parse_args()
nodes = options.nodes

#Creates two lists with the node names in them
group1 = list(range(1, options.sizeg1+1))
group2 = list(range(1, nodes + 1))


for element in group1:
    group2.remove(element)

i = 0

if not options.recombine:
    for _ in range(5):
        i += 1
        x = 0
        for n in range(1, nodes+1):
            nodeingroup2 = False
            y = 200
            if n > options.sizeg1:
                nodeingroup2 = True
                x = x + 100 + i * 10
                if n % 2 == 0:
                    y = 400
            else:
                x = x + 100 - i * 10
                if n % 2 == 0:
                    y = 400
            subprocess.run(["python3.6", coresendmsgpath,
                           "NODE", "NUMBER=%s" %n,  "X_POSITION=%s" %x, "Y_POSITION=%s" % y])
            if nodeingroup2:
                x = x - i * 10
            else:
                x = x + i * 10
else:
    print("recombining")
    i = 6
    for _ in range(5):
        i -= 1
        x = 100*(options.nodes+1)
        for n in range(nodes, 0, -1):
            nodeingroup2 = False
            y = 200
            if n > options.sizeg1:
                nodeingroup2 = True
                x = x - 100 + i * 10
                if n % 2 == 0:
                    y = 400
            else:
                x = x - 100 - i * 10
                if n % 2 == 0:
                    y = 400
            subprocess.run(["python3", coresendmsgpath,
                           "NODE", "NUMBER=%s" %n,  "X_POSITION=%s" %x, "Y_POSITION=%s" % y])
            if nodeingroup2:
                x = x - i * 10
            else:
                x = x + i * 10

#
# process = subprocess.Popen(bashCommand.split(), stdout=subprocess.PIPE)
# output, error = process.communicate()
