#!/usr/bin/env bash

tc qdisc add dev client-eth0 root handle 5:0 hfsc default 1
tc class add dev client-eth0 parent 5:0 classid 5:1 hfsc sc rate 30Mbit ul rate 35Mbit

tc qdisc add dev client-eth1 root handle 5:0 hfsc default 1
# tc class add dev client-eth1 parent 5:0 classid 5:1 hfsc sc rate 10Mbit ul rate 10Mbit
tc class add dev client-eth1 parent 5:0 classid 5:1 hfsc sc rate 55Mbit ul rate 60Mbit

# tc qdisc add dev client-eth0 parent 5:1 netem delay 10ms
# tc qdisc add dev client-eth1 parent 5:1 netem delay 100ms

# tc qdisc add dev client-eth0 parent 5:1 netem loss 1.0% 50% delay 10ms 1ms distribution pareto
# tc qdisc add dev client-eth1 parent 5:1 netem loss 1.0% 50% delay 100ms 1ms distribution pareto

tc qdisc add dev client-eth0 parent 5:1 netem delay 10ms
tc qdisc add dev client-eth1 parent 5:1 netem delay 15ms