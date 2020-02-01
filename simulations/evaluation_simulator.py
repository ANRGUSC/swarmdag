import itertools
import random
import simpy
import numpy as np
import networkx as nx
import uuid
from datetime import datetime
import matplotlib.pyplot as plt
import matplotlib
from pprint import pprint

import matplotlib.pylab as pylab
params = {'legend.fontsize': 'x-large',
          'figure.figsize': (16, 8),
         'axes.labelsize': 30,
         'axes.titlesize':'x-large',
         'xtick.labelsize':30,
         'ytick.labelsize':30}
pylab.rcParams.update(params)
matplotlib.rcParams['ps.useafm'] = True
matplotlib.rcParams['pdf.use14corefonts'] = True
matplotlib.rcParams['text.usetex'] = True

class BroadcastPipe(object):
    """A Broadcast pipe that allows one process to send messages to many.

    This construct is useful when message consumers are running at
    different rates than message generators and provides an event
    buffering to the consuming processes.

    The parameters are used to create a new
    :class:`~simpy.resources.store.Store` instance each time
    :meth:`get_output_conn()` is called.

    """
    def __init__(self, env, capacity=simpy.core.Infinity):
        self.env = env
        self.capacity = capacity
        self.pipes = []

    def put(self, value):
        """Broadcast a *value* to all receivers."""
        if not self.pipes:
            raise RuntimeError('There are no output pipes.')
        events = [store.put(value) for store in self.pipes]
        return self.env.all_of(events)  # Condition event for all "events"

    def get_output_conn(self):
        """Get a new output connection for this broadcast pipe.

        The return value is a :class:`~simpy.resources.store.Store`.

        """
        pipe = simpy.Store(self.env, capacity=self.capacity)
        self.pipes.append(pipe)
        return pipe

def net_split(partitions):
    global NUM_NODES

    if len(partitions) == NUM_NODES:
        # network split not possible
        return


    while True:
        p = random.choice(partitions)
        if len(p['nodes']) > 1:
            break

    new_partition = {'id': get_partition_id(), 'nodes': []}

    for iter in range(0, random.randint(1, len(p['nodes']) - 1)):
        new_partition['nodes'].append(p['nodes'].pop(random.randrange(len(p['nodes']))))

    partitions.append(new_partition)
    p['id'] = get_partition_id()
    return 

def net_merge(partitions):
    global NUM_NODES
    global total_tx
    global tx_queue
    global total_delay
    global num_delays
    global avg_delay
    global delta
    global diff_squared_sum
    global all_delays

    if len(partitions) == 1:
        # network merge not possible
        return

    p = []
    p.append(partitions.pop(random.randrange(len(partitions))))
    p.append(partitions.pop(random.randrange(len(partitions))))

    new_partition_nodes = p[0]['nodes'] + p[1]['nodes']
    new_partition_nodes.sort()
    new_partition = {'id': get_partition_id(), 'nodes': new_partition_nodes}
    partitions.append(new_partition)

    if SPECIFIC_NODES_REQUIRED:
        #check queues if any partitions are valid now
        for node in new_partition_nodes:
            if tx_queue[node]:
                leftover_queue = []
                for tx in tx_queue[node]:
                    if all(n in new_partition_nodes for n in tx['nodes_reqd']) == True:
                        tx_delay = env.now - tx['time']

                        # if tx_delay > 300:
                        #     continue 

                        all_delays.append(tx_delay)
                        total_delay += tx_delay
                        num_delays += 1
                        total_tx += 1
                        # delta = tx_delay - avg_delay 
                        # avg_delay += delta / num_delays
                        # diff_squared_sum += delta * (tx_delay - avg_delay)
                    else:
                        leftover_queue.append(tx)

                tx_queue[node] = leftover_queue 
                del leftover_queue

    elif NUMBER_NODES_REQUIRED:
        pass
        


def create_tx(env, partitions):
    global tx_queue
    global total_tx
    global NUM_NODES

    if len(partitions) == 1:
        # any transaction is possible
        total_tx += 1
        return

    node_id = random.randint(0, NUM_NODES - 1)
    for p in partitions:
        for n in p['nodes']:
            if n == node_id:
                if np.random.binomial(1, IN_PARTITION_PROB):
                    total_tx += 1
                    return
                else:
                    #transaction requires robots in other partitions, so generate
                    #the partition requirement for this tx to go through

                    # option 1: require a certain number of nodes beyond partition size
                    num_nodes_reqd =  random.randint(len(p['nodes']) + 1, NUM_NODES)

                    # option 2: require specific nodes with at least one being outside
                    total_nodes_reqd = random.randint(1, NUM_NODES)
                    nodes_reqd = random.sample(range(NUM_NODES), total_nodes_reqd)

                    if all(i in p['nodes'] for i in nodes_reqd):
                        # make at least one required node be outside of the current partition
                        nodes_reqd.pop()
                        r = None
                        while r in p['nodes'] or r is None:
                            r = random.randint(0, NUM_NODES - 1)
                        nodes_reqd.append(r)

                    nodes_reqd.sort()

                    tx_queue[node_id].append({'time': env.now, 
                        'partition_id': p['id'], 
                        'nodes_reqd': nodes_reqd, 'num_nodes_reqd': num_nodes_reqd})

                    # p['nodes'].sort()
                    # nodes_reqd.sort()
                    # print("partition: ", p['nodes'])
                    # print("num nodes reqd: ", num_nodes_reqd)
                    # print("actual nodes reqd: ", nodes_reqd) 

                return
    print("create_tx: should not reach this.")

def message_generator(name, env, out_pipe, rate):
    """A process which randomly generates messages."""
    while True:
        # wait for next transmission
        timeo = random.expovariate(rate)
        yield env.timeout(timeo)

        # if name == 'SPLIT':
        #     print("interval", timeo)V

        # messages are time stamped to later check if the consumer was
        # late getting them.  Note, using event.triggered to do this may
        # result in failure due to FIFO nature of simulation yields.
        # (i.e. if at the same env.now, message_generator puts a message
        # in the pipe first and then message_consumer gets from pipe,
        # the event.triggered will be True in the other order it will be
        # False
        msg = (env.now, name)
        out_pipe.put(msg)


def message_consumer(name, env, in_pipe, partitions):
    global total_delay
    global num_delays

    global tx_queue
    """A process which consumes messages."""
    while True:
        # Get event for message pipe
        msg = yield in_pipe.get()

        # message_consumer is synchronized with message_generator
        # print('at time %.4f: %5s event consumed by %s.' % (env.now, msg[1], name))

        if msg[1] == 'Tx':
            # print('at time %.4f: %5s event' % (env.now, msg[1]))
            create_tx(env, partitions)
        elif msg[1] == 'MERGE':
            # print('at time %.4f: %5s event' % (env.now, msg[1]))
            net_merge(partitions)
            # print("**MERGE** at", env.now, partitions)
        elif msg[1] == 'SPLIT':
            # print('at time %.4f: %5s event' % (env.now, msg[1]))
            net_split(partitions)
            # print("**SPLIT** at", env.now, partitions)

        # Process does some other work, which may result in missing messages
        # yield env.timeout(random.randint(4, 8))

def get_partition_id():
    global partition_id
    partition_id = partition_id + 1
    return partition_id


SIM_TIME = 86400 #1800 is 30 min, 86400 is 1 day, 604800 is 1 week
NUM_NODES = 30

# arrival rates (300 sec in 5 min) 
LAMBDA_SPLIT = 1.0/120
LAMBDA_MERGE = 1.0/60 #once per 5 min
LAMBDA_TX    = 1 * NUM_NODES

IN_PARTITION_PROB = 0.9 #1.0 - (1.0 / (68.5714 * NUM_NODES))
SPECIFIC_NODES_REQUIRED = 1
NUMBER_NODES_REQUIRED = 0

partition_id = -1

# element i in the list is node i's pending due to partition queue
tx_queue = [[] for _ in range(0, NUM_NODES)]
total_tx = 0
all_delays = []
total_delay = 0
num_delays = 0
avg_delay = 0.0
delta = 0.0
diff_squared_sum = 0.0




# start
print('SwarmDAG Emulator')

# start with the entire swarm network
partitions = [{'id': get_partition_id(), 'nodes': list(range(0, NUM_NODES))}]
random.seed(datetime.now())

env = simpy.Environment()

# For one-to-one or many-to-one type pipes, use Store
pipe = simpy.Store(env)
env.process(message_generator('Tx', env, pipe, LAMBDA_TX))
env.process(message_generator('SPLIT', env, pipe, LAMBDA_SPLIT))
env.process(message_generator('MERGE', env, pipe, LAMBDA_MERGE))
env.process(message_consumer('Event Dispatcher', env, pipe, partitions))

env.run(until=SIM_TIME)


if num_delays > 0:
    print("***************************")
    print("split_rate:", LAMBDA_SPLIT)
    print("num_delays:", num_delays)
    print("average delay: %0.2f" % (float(total_delay)/float(num_delays)), "seconds")
    print("std dev: %0.2f" % np.std(all_delays))
else:
    print("No delays occurred.")

# print('Final Statistics:')
# print('total_tx: %d' % total_tx)

tx_queue_len = 0
for l in tx_queue:
    tx_queue_len = tx_queue_len + len(l)

# print("tx/sec: ", float(total_tx)/SIM_TIME)
fig = plt.figure()

plt.hist(all_delays, bins=[i * 50 for i in range(28)])

with open('all_delays.txt', 'w') as f:
    for item in all_delays:
        f.write("%s\n" % item)

plt.xlim([0,1400])
plt.xlabel('Delay (s)')
plt.ylabel('Number of Tx')
plt.title(r'$\lambda_{merge}=\frac{1}{60}, \lambda_{split}=\frac{1}{120}, N_{nodes}=30, p_{queue}^{Tx}=0.1$', fontsize=30)
plt.tight_layout()
fig.savefig('figures/tx_delay_hist_1-120.eps')
fig.savefig('figures/tx_delay_hist_1-120.png')
# plt.show()


# Example Code for SimPy
# For one-to many use BroadcastPipe
# (Note: could also be used for one-to-one,many-to-one or many-to-many)
# env = simpy.Environment()
# bc_pipe = BroadcastPipe(env)

# env.process(message_generator('Generator A', env, bc_pipe))
# env.process(message_consumer('Consumer A', env, bc_pipe.get_output_conn()))
# env.process(message_consumer('Consumer B', env, bc_pipe.get_output_conn()))

# print('\nOne-to-many pipe communication\n')
# env.run(until=SIM_TIME)