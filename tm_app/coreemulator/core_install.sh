#!/bin/bash
echo downloading
cd Desktop
mkdir CoreInstalation
cd CoreInstalation
wget https://github.com/coreemu/core/releases/download/release-5.4.0/core_python3_5.4.0_amd64.deb
wget https://github.com/coreemu/core/releases/download/release-5.4.0/requirements.txt
echo Loading python3 libraries.
python3 -m pip install -r requirements.txt
echo installing
apt install -y ./core_python3_5.4.0_amd64.deb

echo Instructions
echo Run core daemon first by running /etc/init.d/core-daemon start
echo Start core-gui by running core-gui
echo Create the config for SwarmDag by running createNodeLine.py or createNodeGrid.py
echo createNodeGrid.py creates a grid of 8 nodes. Can change this by using an arg. For example, running python3 createNodeGrid.py -n 4 for 4 nodes.
echo createNodeLine.py creates a line of 8 nodes, but can be changed by running python3 createNodeLine.py -n 4 for 4 nodes.
echo
echo To run scripts, double click on a node on the gui. This will open up a terminal
echo Go to your root directory by typing in cd /
echo from there, locate your executable and run it. 
echo 
echo to automatically move the nodes in the grid formation, run ModeNodes.py.
echo its arguments when running are 
echo -n int   for number of nodes (default 8)
echo -s int    size of first group (default 4)
echo -r True or False Recombine the nodes (Default False, meaning it will split the two groups apart.)
