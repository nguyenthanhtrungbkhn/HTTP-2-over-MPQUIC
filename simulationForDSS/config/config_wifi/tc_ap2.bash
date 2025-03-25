#!/usr/bin/env bash
tc qdisc del root dev ap2-eth2

tc qdisc add dev ap2-eth2 root handle 5:0 hfsc default 1
tc class add dev ap2-eth2 parent 5:0 classid 5:1 hfsc sc rate 55Mbit ul rate 60Mbit
# tc qdisc add dev ap2-eth2 parent 5:1 netem delay 15ms

# tc qdisc add dev ap2-eth2 parent 5:1 netem delay 15ms
# tc qdisc add dev ap2-eth2 parent 5:1 netem loss 1.5% 50% delay 15ms 1.2ms 75%
tc qdisc add dev ap2-eth2 parent 5:1 netem loss 3% 50% delay 15ms 2.4ms 75%