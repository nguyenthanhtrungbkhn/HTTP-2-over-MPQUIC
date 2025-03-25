#!/usr/bin/env bash
tc qdisc del root dev ap1-eth2

tc qdisc add dev ap1-eth2 root handle 5:0 hfsc default 1
tc class add dev ap1-eth2 parent 5:0 classid 5:1 hfsc sc rate 30Mbit ul rate 35Mbit
# tc qdisc add dev ap1-eth2 parent 5:1 netem delay 10ms

tc qdisc add dev ap1-eth2 parent 5:1 netem loss 1.5% 50% delay 10ms 0.8ms 75%