#! /bin/sh

tmux new-session -s "0to3" -d 'tail -f build/swarmdag0.log'
tmux split-window -v 'tail -f build/swarmdag1.log'
tmux split-window -h 'tail -f build/swarmdag3.log'
tmux select-pane -t 0
tmux split-window -h 'tail -f build/swarmdag2.log'
tmux -2 attach-session -t "0to3"