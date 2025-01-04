# A framework for HTTP/2 over MPQUIC: Design and Implementation
## Requirements
- go version go1.20 linux/amd64
- python 3.10
- Mininet-WiFi (https://mininet-wifi.github.io)

## Contributions
- Framework Design and Implementation: We introduce a novel framework for integrating HTTP/2 over MPQUIC, tackling key challenges in stream scheduling and protocol compatibility. Within this framework, we implement four stream schedulers: Round Robin (RR), Weighted Round Robin (WRR), Scattered Weighted Round Robin (sWRR), and a newly proposed data size-based WRR (dWRR). 
- Experimental Evaluation: We thoroughly evaluate these schedulers using our framework in Mininet-WiFi. 

## Automative test
- Run simulationForDSS/wifi_test_script.sh

## Manual setup

- Build MPQUIC client and server: 
```go build . ```
```go install ./...```

- If Mininet crashes for some reason, clean it up:
    > sudo mn -c

## Output
- simulationForDSS/output/...client.logs : Client logs
- simulationForDSS/output/...result.csv : Download time 
- simulationForDSS/output/...server.logs : Server logs
- simulationForDSS/output/...time.csv : Object completion time
- simulationForDSS/output/...byte.csv : Object completion by byte
