#!/usr/bin/env bash
tc qdisc del root dev r0-eth1
tc qdisc del root dev r0-eth2

tc qdisc add dev r0-eth1 root handle 5:0 hfsc default 1
tc class add dev r0-eth1 parent 5:0 classid 5:1 hfsc sc rate 30Mbit ul rate 35Mbit
tc qdisc add dev r0-eth2 root handle 5:0 hfsc default 1
tc class add dev r0-eth2 parent 5:0 classid 5:1 hfsc sc rate 55Mbit ul rate 60Mbit

# Base delay
tc qdisc add dev r0-eth1 parent 5:1 netem delay 10ms
tc qdisc add dev r0-eth2 parent 5:1 netem delay 15ms 

# tc qdisc add dev r0-eth1 parent 5:1 netem loss 1.5% 50% delay 10ms 0.8ms 75%

# tc qdisc add dev r0-eth2 parent 5:1 netem delay 15ms
# tc qdisc add dev r0-eth2 parent 5:1 netem loss 1.5% 50% delay 15ms 1.2ms 75%
# tc qdisc add dev r0-eth2 parent 5:1 netem loss 3% 50% delay 15ms 2.4ms 75%