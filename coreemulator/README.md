## Scripts for setting up core emulator and controlling the nodes.

## Instructions

### CORE
Version 7.3.0

Follow "Automated Install" directions to install into a virtual env. Use `inv`
to install EMANE to the virtual environment.

    core-python -m pip install requirements.txt

### Poetry (TODO)

### Launching:
 Run python script by using

    sudo core-python partition.py

Ctrl+c to stop, and then cleanup:

    sudo core-cleanup


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
