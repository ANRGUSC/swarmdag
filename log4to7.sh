#! /bin/sh

tmux new-session -s "4to7" -d 'tail -f build/swarmdag4.log'
tmux split-window -v 'tail -f build/swarmdag5.log'
tmux split-window -h 'tail -f build/swarmdag7.log'
tmux select-pane -t 0
tmux split-window -h 'tail -f build/swarmdag6.log'
tmux -2 attach-session -t "4to7"