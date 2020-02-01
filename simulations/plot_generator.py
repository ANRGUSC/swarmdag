

# For plotting the statistics wrt angle varaition for 3m 

import csv
import os 
import sys
import numpy as np
import math
import matplotlib.pyplot as plt
import matplotlib
import math

from pprint import *

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

num_nodes = [10, 20, 30, 40, 50]

# split rate 1/600
avg_delay_1_600 = [69.97, 71.684, 76.03, 72.6, 74.55]
num_samples_1_600 = [117658, 205373, 27163, 34016, 42972]
std_dev_1_600 = [71.35, 72.07, 83.29, 74.33, 78.13]

#split rate 1/240
avg_delay_1_240 = [96.33, 99.6, 105.86, 98.11, 100.82]
num_samples_1_240 = [34067, 42836, 64742, 82128, 109023]
std_dev_1_240 = [108.03, 117.82, 124.44, 113.46, 109.38]

#split rate 1/120
avg_delay_1_120 = [199.1, 207.79, 217.75, 235.51, 236.41]
num_samples_1_120 = [43578, 84015, 128358, 170123, 212051]
std_dev_1_120 = [251.02, 266.08, 281.62, 329.18, 296.5]


err_1_600 = []
err_1_240 = []
err_1_120 = []
for i  in range(0, 5):
    err_1_600.append(10* std_dev_1_600[i] / math.sqrt(num_samples_1_600[i]))
    err_1_240.append(10* std_dev_1_240[i] / math.sqrt(num_samples_1_240[i]))
    err_1_120.append(10* std_dev_1_120[i] / math.sqrt(num_samples_1_120[i]))


split_rates = [1/600, 1/540, 1/480, 1/420, 1/360, 1/300, 1/240, 1/180, 1/120, 1/90]

avg_delay_vary_splitrate = [73.85, 74.20789155,76.74786235, 81.24856876, 82.24305437, 89.26883907, 100.8583148, 132.7846137, 209.4000337, 426.9220199]

num_samples_vary_splitrate = [22399, 28073, 31840, 37369, 43442, 43442, 50107, 89037, 127964, 168985]
std_dev_vary_splitrate = [72.07, 74.77833333, 85.28875, 84.5175, 89.175, 107.23375, 117.5375, 164.30125, 289.97625, 598.0733333]

err_vary_splitrate = []
for i  in range(0, len(split_rates)):
    err_vary_splitrate.append(30 * std_dev_vary_splitrate[i] / math.sqrt(num_samples_vary_splitrate[i]))


num_nodes_ptxq = [10, 30, 50]
prob_tx_queued = [0.1, 0.3, 0.5]
avg_delay_n_10 = [95.37849697, 97.44630922, 105.4300237]
num_samples_n_10 = [20811.33333, 63295.86667, 110174.3333]
std_dev_n_10 = [122.7675, 102.124, 112.745]

avg_delay_n_30 = [100.86, 103.92, 108.66]
num_samples_n_30 = [64885, 190819, 325222]
std_dev_n_30 = [117.5, 127, 120.44]

avg_delay_n_50 = [101.5779463, 109.9088476, 113.8365794]
num_samples_n_50 = [109086.2, 328650.8667, 540962.7333]
std_dev_n_50 = [109.3825, 132.07, 133.27]

err_ptxq_n_10 = []
err_ptxq_n_30 = []
err_ptxq_n_50 = []
for i  in range(0, len(num_nodes_ptxq)):
    err_ptxq_n_10.append(5 * std_dev_n_10[i] / math.sqrt(num_samples_n_10[i]))
    err_ptxq_n_30.append(10 * std_dev_n_30[i] / math.sqrt(num_samples_n_30[i]))
    err_ptxq_n_50.append(10 * std_dev_n_50[i] / math.sqrt(num_samples_n_50[i]))

fig = plt.figure()
plt.errorbar(num_nodes, avg_delay_1_600, yerr=err_1_600, fmt='o-', label=r'$\lambda_{split}=\frac{1}{600}$', markersize=20, linewidth=4)
plt.errorbar(num_nodes, avg_delay_1_240, yerr=err_1_240, fmt='o-', label=r'$\lambda_{split}=\frac{1}{240}$', markersize=20, linewidth=4)
plt.errorbar(num_nodes, avg_delay_1_120, yerr=err_1_120, fmt='o-', label=r'$\lambda_{split}=\frac{1}{120}$', markersize=20, linewidth=4)
plt.xlabel('Network Size $N_{nodes}$')
plt.ylabel('Average Delay (s)')
plt.title(r'$\lambda_{merge}=\frac{1}{60}, \lambda_{Tx}= 1~Tx/s/node, p_{queue}^{Tx}=0.1$', fontsize=30)
plt.grid(True)
fig.legend(loc = 'center right', fontsize = 30)
plt.gcf().subplots_adjust(bottom=0.15)
# plt.show()
fig.savefig('figures/avg_delay_vs_netsize.eps')
fig.savefig('figures/avg_delay_vs_netsize.png')


# fig1 = plt.figure()
fig1, ax = plt.subplots()
lambda_split = [
r'$\frac{1}{600}$', 
r'$\frac{1}{540}$', 
r'$\frac{1}{480}$', 
r'$\frac{1}{420}$', 
r'$\frac{1}{360}$', 
r'$\frac{1}{300}$', 
r'$\frac{1}{240}$', 
r'$\frac{1}{180}$', 
r'$\frac{1}{120}$', 
r'$\frac{1}{90}$']
plt.errorbar(split_rates, avg_delay_vary_splitrate, yerr=err_vary_splitrate, fmt='o-', markersize=20, linewidth=4)
for i, rate in enumerate(lambda_split):
    ax.annotate(rate,xy = ([split_rates[i], avg_delay_vary_splitrate[i] + 10]), fontsize=35)
plt.xlabel('Network Split Rate $\lambda_{split}$')
plt.ylabel('Average Delay (s)')
plt.title(r'$\lambda_{merge}=\frac{1}{60}, N_{nodes}=30, \lambda_{Tx}=1~Tx/s/node$', fontsize=30)
plt.grid(True)
plt.xscale("log")
plt.tight_layout()
# plt.show()
fig1.savefig('figures/avg_delay_vs_splitrate.eps')
fig1.savefig('figures/avg_delay_vs_splitrate.png')

fig2 = plt.figure()
plt.errorbar([0.1, 0.3, 0.5], avg_delay_n_10, yerr=err_ptxq_n_10, fmt='o-', label='$N_{nodes}=10$', markersize=20, linewidth=4)
plt.errorbar([0.1, 0.3, 0.5], avg_delay_n_30, yerr=err_ptxq_n_30, fmt='o-', label='$N_{nodes}=30$', markersize=20, linewidth=4)
plt.errorbar([0.1, 0.3, 0.5], avg_delay_n_50, yerr=err_ptxq_n_50, fmt='o-', label='$N_{nodes}=50$', markersize=20, linewidth=4)
plt.ylabel('Average Delay (s)')
plt.xlabel('Prob. Transaction is Queued $p_{queued}^{Tx}$')
plt.title(r'$\lambda_{merge}=\frac{1}{60}, \lambda_{split}=\frac{1}{240}, \lambda_{Tx}= 1~Tx/s/node$', fontsize=30)
plt.grid(True)
fig2.legend(bbox_to_anchor = (0.9, 0.3), loc = 'center right', fontsize = 30)
plt.gcf().subplots_adjust(bottom=0.15)
# plt.show()
fig2.savefig('figures/avg_delay_vs_ptxq.eps')
fig2.savefig('figures/avg_delay_vs_ptxq.png')



# fig1 = plt.figure()
# plt.errorbar(data[d_org], angle_e_mean[d_org], yerr=angle_e_std[d_org], fmt='o', markersize=20, linewidth=4)
# plt.xlabel('Angle (degrees)')
# plt.ylabel('Angle Estimation Error (degrees)')
# plt.grid(True)
# fig1.savefig('figures/angle_err_3m.eps')
# # plt.plot(data[d_org], angle_e_mean[d_org])
# plt.show()