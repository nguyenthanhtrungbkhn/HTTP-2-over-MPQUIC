#!/bin/bash
#!/bin/sleep
#!/bin/sh

sudo mn -c
# cd /home/"$(whoami)"/go/src/github.com/lucas-clemente/quic-go ; go build ; go install ./...
# cd /home/"$(whoami)"/go/src/github.com/lucas-clemente/simulationForDSS
# pwd
# cp /home/"$(whoami)"/go/bin/example ./serverMPQUIC
# cp /home/"$(whoami)"/go/bin/client_benchmarker ./clientMPQUIC

sudo rm ./logs/*
# sudo rm ./output/result-wireless/*

file_path="www/listwebsite.txt"

#initial: array stores web list
# declare -a webArr=("google.com")
# declare -a webArr=("abc.com")

#read file and store to arr
while IFS= read -r line || [ -n "$line" ]; do
    line=$(echo "$line" | sed 's/^https:\/\///')
    webArr+=("$line")
done < "$file_path"

#echo "Print arr store folder website:"
for web in "${webArr[@]}"; do
    echo "$web"
done

declare -a mdlArr=("none")
declare -a numArr=("1")
# declare -a filArr=("1MB") #tranfer to webArr
declare -a bgrArr=("0") #background traffic
declare -a frqArr=("0") #frq
declare -a bwdArr=("0") #bandwidth
declare -a owdArr=("0") #one-way delay
declare -a varArr=("16") #variation delay
declare -a losArr=("3") #pkt loss 
declare -a brsArr=("firefox" "safari" "chrome")

declare -a schArr=("SAECF")
declare -a stmArr=("WRR")
for mdl in "${mdlArr[@]}"
do 
    for num in "${numArr[@]}"
    do 
        for fil in "${webArr[@]}"
        do
            for bgr in "${bgrArr[@]}"
            do 
                for frq in "${frqArr[@]}"
                do 
                    for bwd in "${bwdArr[@]}"
                    do 
                        for owd in "${owdArr[@]}"
                        do
                            for var in "${varArr[@]}"
                            do
                                for los in "${losArr[@]}"
                                do 
                                    for sch in "${schArr[@]}"
                                    do
                                        for stm in "${stmArr[@]}"
                                        do 
                                            for brs in "${brsArr[@]}"
                                            do 
                                                echo "$mdl-$num-$fil-$bgr-$frq-$bwd-$owd-$var-$los-$sch-$stm-$brs"
                                                sudo python wifi_scenario.py --model ${mdl} --client ${num} --website ${fil} --background ${bgr} --frequency ${frq} --bandwidth ${bwd} --delay ${owd} --variation ${var} --loss ${los} --scheduler ${sch} --stream ${stm} --model ${mdl} --browser ${brs}
                                                sudo mv ./logs/server.logs ./output/${mdl}-${num}-${fil}-${bgr}-${frq}-${bwd}-${owd}-${var}-${los}-${sch}-${stm}-${brs}-server.logs
                                                sudo mv ./logs/client.logs ./output/${mdl}-${num}-${fil}-${bgr}-${frq}-${bwd}-${owd}-${var}-${los}-${sch}-${stm}-${brs}-client.logs
                                                sudo mv ./logs/data-time.csv ./output/${mdl}-${num}-${fil}-${bgr}-${frq}-${bwd}-${owd}-${var}-${los}-${sch}-${stm}-${brs}-time.csv   
                                                sudo mv ./logs/data-byte.csv ./output/${mdl}-${num}-${fil}-${bgr}-${frq}-${bwd}-${owd}-${var}-${los}-${sch}-${stm}-${brs}-byte.csv   
                                                sudo mv ./logs/result.csv ./output/${mdl}-${num}-${fil}-${bgr}-${frq}-${bwd}-${owd}-${var}-${los}-${sch}-${stm}-${brs}-result.csv   
                                                # sudo mv ./logs/server-flask.logs ./output/result-wireless/${mdl}-${num}-${fil}-${bgr}-${frq}-${bwd}-${owd}-${var}-${los}-${sch}-${stm}-${brs}-flask.logs  
                                                # sudo mv ./logs/training_history.png ./output/result-wireless/${mdl}-${num}-${fil}-${bgr}-${frq}-${bwd}-${owd}-${var}-${los}-${sch}-${stm}-${brs}-training.png 
                                                sudo mn -c
                                                sleep 10
                                            done
                                        done
                                    done
                                done
                            done
                        done
                    done
                done
            done
        done
    done
done

