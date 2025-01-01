#!/usr/bin/env python

'Setting the position of nodes and providing mobility'

import sys
import time
import argparse
from threading import Thread
import threading
import os
import random
from mininet.log import setLogLevel, info
from mininet.node import Controller, Node
from mn_wifi.net import Mininet_wifi
from mn_wifi.cli import CLI
from mn_wifi.wmediumdConnector import interference
from mn_wifi.link import wmediumd
from mn_wifi.node import Station

SERVER_CMD = "./serverMPQUIC"
CERTPATH = "--certpath ./quic/quic_go_certs"
SCH = "-ps %s"
STH = "-ss %s"
ARGS = "-bind :6121 -www ./www/"
END = ">> ./logs/server.logs 2>&1"

with_background = 0  # Global variable to control the creation of background traffic

def get_file(directory_path):
    file_list = []
    for path, subdirs, files in os.walk(directory_path):
        for name in files:
            tmp = os.path.join(path, name)
            tmp_t = tmp.replace(directory_path,"")
            file_list.append(tmp_t)
    return file_list

def get_web(website):
    cwd = os.getcwd()
    directory_path = cwd + "/www/source/"+website
    # print(directory_path)
    files = get_file(directory_path)
    for i in range(len(files)):
        files[i] = "https://10.0.0.20:6121/source/"+website + files[i]

    list_ds_w = ' '.join(files)
    with open('list_w.txt', 'w') as f:
        for line in files:
            f.write(line)
            f.write('\n')
    # print(list_ds_w)

CLIENT_CMD = "./clientMPQUIC -m"
CLIENT_SCH = "-ps %s"
CLIENT_STH = "-ss %s"
CLIENT_BRS = "-bs %s list_w.txt"
CLIENT_END = ">> ./logs/client.logs 2>&1"

class LinuxRouter(Node):
    def config( self, **params ):
        super( LinuxRouter, self).config( **params )
        self.cmd( 'sysctl net.ipv4.ip_forward=1' )

    def terminate( self ):
        self.cmd( 'sysctl net.ipv4.ip_forward=0' )
        super( LinuxRouter, self ).terminate()

def runClient(station, id, client_cmd):
    for i in range(100):
        time.sleep(1)
        station.sendCmd(client_cmd.format(id=id))

        # Timeout of 20 seconds for detecting crashing tests
        output = station.monitor(timeoutms=20000)

def configClient(sta, id):
    sta.cmd("ifconfig sta{id}-wlan0 down".format(id=id))
    sta.cmd("ip link set sta{id}-wlan0 name wlan0".format(id=id))  
    sta.cmd("ifconfig wlan0 up")

    sta.cmd("ifconfig sta{id}-wlan1 down".format(id=id))
    sta.cmd("ip link set sta{id}-wlan1 name wlan1".format(id=id))  
    sta.cmd("ifconfig wlan1 up")

    sta.cmd("ifconfig wlan0 192.168.2.{id} netmask 255.255.255.0".format(id=id))
    sta.cmd("ifconfig wlan1 172.16.0.{id} netmask 255.240.0.0".format(id=id))

    sta.cmd("ip rule add from 192.168.2.{id}/24 table 1".format(id=id))
    sta.cmd("ip rule add from 172.16.0.{id}/12 table 2".format(id=id))

    sta.cmd("ip route add default nexthop via 192.168.2.2 dev wlan0 weight 1 nexthop via 172.16.0.2 dev wlan1 weight 1")
    sta.cmd("ip route add default via 192.168.2.2 table 1")
    sta.cmd("ip route add default via 172.16.0.2 table 2")


def topology(args, server_cmd, model, clt, client_cmd):

    net = Mininet_wifi(controller=Controller, link=wmediumd, wmediumd_mode=interference, fading_cof=3)

    info("*** Creating nodes\n")
    h1 = net.addHost('h1', mac='00:00:00:00:00:01', ip='10.0.0.20/8', defaultRoute='10.0.0.2')
    s1 = net.addSwitch('s1', mac='00:00:00:00:00:02')
    r0 = net.addHost( 'r0', cls=LinuxRouter, ip='192.168.2.2/24')
    #mode a,n,ac (channel 36,40), mode g (channel 1,5,6)
    ap1 = net.addAccessPoint('ap1', mac='00:00:00:00:00:04', ssid='lte-ssid', mode='a', channel='36', position='45,50,0')
    ap2 = net.addAccessPoint('ap2', mac='00:00:00:00:00:05', ssid='wifi-ssid', mode='a', channel='40', position='55,50,0')

    if (model == 'none'):
        sta3 = net.addStation('sta3', wlans=2, mac='00:00:00:00:01:03', position='50,70,0')
    else:
        sta3 = net.addStation('sta3', wlans=2, mac='00:00:00:00:01:03')

    # sta4 = net.addStation('sta4', wlans=2, mac='00:00:00:00:01:04', position='50,70,0')
    # sta5 = net.addStation('sta5', wlans=2, mac='00:00:00:00:01:05', position='50,70,0')
    # sta6 = net.addStation('sta6', wlans=2, mac='00:00:00:00:01:06', position='50,70,0')
    # sta7 = net.addStation('sta7', wlans=2, mac='00:00:00:00:01:07', position='50,70,0')
    # sta8 = net.addStation('sta8', wlans=2, mac='00:00:00:00:01:08', position='50,70,0')
    # sta9 = net.addStation('sta9', wlans=2, mac='00:00:00:00:01:0A', position='50,70,0')
    # sta10 = net.addStation('sta10', wlans=2, mac='00:00:00:00:01:0B', position='50,70,0')

    c1 = net.addController('c1')

    info("*** Configuring Propagation Model\n")
    net.setPropagationModel(model="logDistance", exp=3)

    info("*** Configuring wifi nodes\n")
    net.configureWifiNodes()

    info("*** Associating and Creating links\n")
    net.addLink( h1, s1, bw=300, use_hfsc = True)

    net.addLink( ap1, r0, intfName2='r0-eth1', use_hfsc = True, params2={ 'ip' : '192.168.2.2/24' } )
    net.addLink( ap2, r0, intfName2='r0-eth2', use_hfsc = True, params2={ 'ip' : '172.16.0.2/12' } )
    net.addLink( s1, r0, intfName2='r0-eth3', use_hfsc = True, params2={ 'ip' : '10.0.0.2/8' } )

    # if '-p' not in args:
    #     net.plotGraph()
    if (model != 'none'):
        net.setMobilityModel(time=0, model=model, max_x=100, max_y=100, seed=20,
                            min_v=2, max_v=4, velocity=(2., 4.), FL_MAX=200.,
                            alpha=0.5, variance=4.)
    info("*** Starting network\n")
    net.build()

    configClient(sta3, 3)
    # configClient(sta4, 4)
    # configClient(sta5, 5)
    # configClient(sta6, 6)
    # configClient(sta7, 7)
    # configClient(sta8, 8)
    # configClient(sta9, 9)
    # configClient(sta10, 10)

    h1.cmd('ip route add default via 10.0.0.2')

    c1.start()
    ap1.start([c1])
    ap2.start([c1])
    s1.start([c1])

    r0.cmd("./config/config_wifi/tc_r0.bash")
    ap1.cmd("./config/config_wifi/tc_ap1.bash")
    ap2.cmd("./config/config_wifi/tc_ap2.bash")
    # varrate = float(args.owd) * float(args.var) / 100
    # r0.cmd('tcdel r0-eth1 --all')
    # r0.cmd('tcset r0-eth1 --rate 30Mbps --delay 10ms --delay-distro 0.8 --delay-distribution pareto --loss 1.5%')
    # r0.cmd('tcset r0-eth2 --rate {}Mbps --delay {}ms --delay-distro {} --delay-distribution pareto --loss {}%'.format(args.bwd, args.owd, varrate, args.los))
    # ap1.cmd('tcset ap1-eth2 --rate 30Mbps --delay 10ms --delay-distro 0.8 --delay-distribution pareto --loss 1.5%')
    # ap2.cmd('tcset ap2-eth2 --rate {}Mbps --delay {}ms --delay-distro {} --delay-distribution pareto --loss {}%'.format(args.bwd, args.owd, varrate, args.los))

    h1.sendCmd(server_cmd)
    # print(clt)
    # CLI(net)
    time.sleep(5)
    if (int(clt) == 1):
        # print("check 1")
        t3 = threading.Thread(target=runClient, args=(sta3,3,client_cmd))
        t3.start()
        t3.join()
    elif (int(clt) == 3):
        # print("check 3")
        t3 = threading.Thread(target=runClient, args=(sta3,3))
        # t4 = threading.Thread(target=runClient, args=(sta4,4))
        # t5 = threading.Thread(target=runClient, args=(sta5,5))

        t3.start()
        # t4.start()
        # t5.start()

        t3.join()
        # t4.join()
        # t5.join()
    elif (int(clt) == 8):
        # print("check 8")
        t3 = threading.Thread(target=runClient, args=(sta3,3))
        # t4 = threading.Thread(target=runClient, args=(sta4,4))
        # t5 = threading.Thread(target=runClient, args=(sta5,5))
        # t6 = threading.Thread(target=runClient, args=(sta6,6))
        # t7 = threading.Thread(target=runClient, args=(sta7,7))
        # t8 = threading.Thread(target=runClient, args=(sta8,8))
        # t9 = threading.Thread(target=runClient, args=(sta9,9))
        # t10 = threading.Thread(target=runClient, args=(sta10,10))

        t3.start()
        # t4.start()
        # t5.start()
        # t6.start()
        # t7.start()
        # t8.start()
        # t9.start()
        # t10.start()

        t3.join()
        # t4.join()
        # t5.join()
        # t6.join()
        # t7.join()
        # t8.join()
        # t9.join()
        # t10.join()
        
    # Check for timeouts
    h1.sendInt()

    h1.monitor()
    h1.waiting = False

    # info("*** Running CLI\n")
    #CLI(net)
    
    info("*** Stopping network\n")
    net.stop()
    time.sleep(1)
    # net.cleanup()

def do_training(args):
    server_cmd = " ".join([SERVER_CMD, CERTPATH, SCH % args.sch, STH % args.stm, ARGS, END])
    client_cmd = " ".join([CLIENT_CMD, CLIENT_SCH % args.sch, CLIENT_STH % args.stm, CLIENT_BRS % args.brs, CLIENT_END])
    setLogLevel('info')
    topology(args, server_cmd, args.mdl, args.clt, client_cmd)


# if __name__ == '__main__':
#     parser = argparse.ArgumentParser(description='Executes a test with defined scheduler')
#     parser.add_argument('--website', dest="web", help="Website dict", required=True)
#     parser.add_argument('--scheduler', dest="sch", help="Scheduler (LowLatency, ECF, SA-ECF)", required=True)
#     parser.add_argument('--stream', dest="stm", help="Stream scheduler (RR, WRR)", required=True)
#     parser.add_argument('--model', dest="mdl", help="Mobility Model, or: none", required=True)
#     parser.add_argument('--client', dest="clt", help="Client Number", required=True)
#     parser.add_argument('--browser', dest="brs", help="Browser Client (safari, firefox, chrome)", required=True)
#     args = parser.parse_args()

#     get_web(args.web)
#     do_training(args.sch, args.stm, args.mdl, args.clt, args.brs)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Executes a test with defined scheduler')
    parser.add_argument('--model', dest="mdl", help="Mobility Model, or: none", required=True)
    parser.add_argument('--client', dest="clt", help="Client Number", required=True)
    parser.add_argument('--bandwidth', dest="bwd", help="bandwidth", required=True)
    parser.add_argument('--delay', dest="owd", help="delay", required=True)
    parser.add_argument('--variation', dest="var", help="variation", required=True)
    parser.add_argument('--loss', dest="los", help="loss", required=True)
    parser.add_argument('--background', dest="bg", help="Enable background traffic (1 or 0)", required=False, default=0)
    parser.add_argument('--frequency', dest="freq", help="Frequency of background traffic (seconds)", required=False, default=1)

    parser.add_argument('--website', dest="web", help="Website dict", required=True)
    parser.add_argument('--scheduler', dest="sch", help="Scheduler (LowLatency, ECF, SA-ECF)", required=True)
    parser.add_argument('--stream', dest="stm", help="Stream scheduler (RR, WRR)", required=True)
    parser.add_argument('--browser', dest="brs", help="Browser Client (safari, firefox, chrome)", required=True)
    args = parser.parse_args()

    get_web(args.web)
    do_training(args)