## Scripts for setting up core emulator and controlling the nodes.

 ## Instructions

 ### Install requirements from requirements.txt in this directory:

    pip3.6 install -r requirements.txt
 
 ### Installing: 
 Core requires python3.6+ and Linux distributions may vary the default version 
 of python when simply using `python3`. To avoid any issues, follow the 
 instructions to build and install core from source. During the `.configure`
 stage, run

    PYTHON=python3.6 ./configure
 
### Launching:
 Run core daemon first by running `/etc/init.d/core-daemon start`
 Start core-gui by running `core-gui`
 
### Creating an instance:
Two options: Create a grid or create a line of nodes.
createNodeGrid.py creates a grid of 8 nodes, but can be changed by running `python3.6 createNodeGrid.py -n 4` for 4 nodes.
createNodeLine.py creates a line of 8 nodes, but can be changed by running `python3.6 createNodeLine.py -n 4` for 4 nodes.

 ### Running Scripts on nodes:
 Double click on a node on the gui. This will open up a terminal. Go to your root directory by typing in `cd /`. Locate your executable and run it. 
 
 ### Automatically moving nodes in the grid formation:
 To automatically move the nodes in the grid formation, run `python3 ModeNodes.py args`.
 its arguments when running are as follows.
 - `-n int`   for number of nodes (default 8)
 - `-s int`    size of first group (default 4)
 - `-r True or False` Recombine the nodes (Default False, meaning it will split the two groups apart.)
