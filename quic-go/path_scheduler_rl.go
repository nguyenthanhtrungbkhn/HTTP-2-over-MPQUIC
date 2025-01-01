package quic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/lucas-clemente/quic-go/go-fuzzy-logic"
	"github.com/lucas-clemente/quic-go/internal/multiclients"
	"github.com/lucas-clemente/quic-go/internal/protocol"
	// "encoding/csv"
	// "os"
)

func NormalizeTimes(stat time.Duration) float64 {
	return float64(stat.Nanoseconds()) / float64(time.Millisecond.Nanoseconds())
}

func NormalizeGoodput(s *session, packetNumber uint64, retransNumber uint64) float64 {
	duration := time.Since(s.sessionCreationTime)

	elapsedtime := NormalizeTimes(duration) / 1000
	goodput := ((float64(packetNumber) - float64(retransNumber)) / 1024 / 1024 / elapsedtime) * float64(protocol.DefaultTCPMSS)

	return goodput
}

func NormalizeRewardGoodput(goodput float64) float64 {
	if goodput < Goodput_Min {
		Goodput_Min = goodput
	}
	if goodput > Goodput_Max {
		Goodput_Max = goodput
	}
	if Goodput_Max > Goodput_Min {
		return (goodput - Goodput_Min) / (Goodput_Max - Goodput_Min)
	} else {
		return 0.0
	}
}

func NormalizeRewardLrtt(lrtt float64) float64 {
	if lrtt < Lrtt_Min {
		Lrtt_Min = lrtt
	}
	if lrtt > Lrtt_Max {
		Lrtt_Max = lrtt
	}
	if Lrtt_Max > Lrtt_Min {
		return (lrtt - Lrtt_Min) / (Lrtt_Max - Lrtt_Min)
	} else {
		return 0.0
	}
}

func (sch *scheduler) GetStateAndRewardFQSAT(s *session, pth *path) {
	lRTT := make(map[protocol.PathID]time.Duration)
	sRTT := make(map[protocol.PathID]time.Duration)

	cwnd := make(map[protocol.PathID]protocol.ByteCount)
	cwndlevel := make(map[protocol.PathID]float32)
	inp := make(map[protocol.PathID]protocol.ByteCount)

	reWard := make(map[protocol.PathID]float64)

	firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)

	for pathID, path := range s.paths {
		//dataString := fmt.Sprintf("%d -", int(pathID))
		//f.WriteString(dataString)
		if pathID != protocol.InitialPathID {
			//packetNumber[pathID], retransNumber[pathID], lostNumber[pathID] = path.sentPacketHandler.GetStatistics()
			lRTT[pathID] = path.rttStats.LatestRTT()
			sRTT[pathID] = path.rttStats.SmoothedRTT()
			cwnd[pathID] = path.sentPacketHandler.GetCongestionWindow()
			inp[pathID] = path.sentPacketHandler.GetBytesInFlight()
			if float32(cwnd[pathID]) != 0 {
				cwndlevel[pathID] = float32(path.sentPacketHandler.GetBytesInFlight()) / float32(cwnd[pathID])
			} else {
				cwndlevel[pathID] = 0
			}

			// Ordering paths
			if firstPath == protocol.PathID(255) {
				firstPath = pathID
			} else {
				if pathID < firstPath {
					secondPath = firstPath
					firstPath = pathID
				} else {
					secondPath = pathID
				}
			}
		}
	}

	//fmt.Println(s.scheduler.AdaDivisor, s.scheduler.Alpha, s.scheduler.Beta)
	reWard[firstPath] = (1-s.scheduler.Alpha)*float64(cwndlevel[firstPath]) - s.scheduler.Alpha*float64(NormalizeTimes(lRTT[firstPath])/50)
	reWard[secondPath] = (1-s.scheduler.Alpha)*float64(cwndlevel[secondPath]) - s.scheduler.Alpha*float64(NormalizeTimes(lRTT[secondPath])/50)

	//sendingRate := (float64(cwnd[pth.pathID])/ float64(lRTT[pth.pathID])) / (float64(cwnd[firstPath])/ float64(lRTT[firstPath]) + float64(cwnd[secondPath])/ float64(lRTT[secondPath]))
	f_sendingRate := (float64(cwnd[firstPath]) / float64(lRTT[firstPath])) / (float64(cwnd[firstPath])/float64(lRTT[firstPath]) + float64(cwnd[secondPath])/float64(lRTT[secondPath]))
	s_sendingRate := (float64(cwnd[secondPath]) / float64(lRTT[secondPath])) / (float64(cwnd[firstPath])/float64(lRTT[firstPath]) + float64(cwnd[secondPath])/float64(lRTT[secondPath]))

	//update Q
	var f_cLevel, s_cLevel, col int8

	if pth.pathID == firstPath {
		col = 0
		if reWard[firstPath] == 0 {
			return
		}
	} else {
		if reWard[secondPath] == 0 {
			return
		}
		col = 1
	}

	if f_sendingRate < sch.clv_state[0] {
		f_cLevel = 0
	} else if f_sendingRate < sch.clv_state[1] {
		f_cLevel = 1
	} else if f_sendingRate < sch.clv_state[2] {
		f_cLevel = 2
	} else if f_sendingRate < sch.clv_state[3] {
		f_cLevel = 3
	} else {
		f_cLevel = 4
	}

	if s_sendingRate < sch.clv_state[0] {
		s_cLevel = 0
	} else if s_sendingRate < sch.clv_state[1] {
		s_cLevel = 1
	} else if s_sendingRate < sch.clv_state[2] {
		s_cLevel = 2
	} else if s_sendingRate < sch.clv_state[3] {
		s_cLevel = 3
	} else {
		s_cLevel = 4
	}

	old_f_cLevel := s.scheduler.currentState_f
	old_s_cLevel := s.scheduler.currentState_s

	var maxNextState float64
	if s.scheduler.qtable[f_cLevel][s_cLevel][0] > s.scheduler.qtable[f_cLevel][s_cLevel][1] {
		maxNextState = s.scheduler.qtable[f_cLevel][s_cLevel][0]
	} else {
		maxNextState = s.scheduler.qtable[f_cLevel][s_cLevel][1]
	}

	var data fuzzy.FuzzyNumber
	data.Family.Number = string(s.scheduler.record)
	data.Family.Debt = float64(cwndlevel[pth.pathID])
	// data.Family.Debt = float64(float32(BSend))
	if sRTT[pth.pathID] != 0 {
		data.Family.Income = math.Abs(float64(lRTT[pth.pathID])/float64(sRTT[pth.pathID]) - 1)
	} else {
		data.Family.Income = 1
	}
	// if float32(packetNumber[pth.pathID]) != 0{
	// 	data.Family.Income = 100.0*float64(float32(lostNumber[pth.pathID]))/float64(float32(packetNumber[pth.pathID]))
	// }else{
	// 	data.Family.Income = 0
	// }
	blt := fuzzy.BLT{}
	blt.Fuzzification(&data)
	blt.Inference(&data)
	blt.Defuzzification(&data)
	s.scheduler.Delta = data.CrispValue

	s.scheduler.record += 1
	// fmt.Println("Reward1")

	newValue := (1-s.scheduler.Delta)*s.scheduler.qtable[old_f_cLevel][old_s_cLevel][col] + (s.scheduler.Delta)*(reWard[pth.pathID]+s.scheduler.Gamma*maxNextState)

	s.scheduler.qtable[old_f_cLevel][old_s_cLevel][col] = newValue
	s.scheduler.currentState_f = f_cLevel
	s.scheduler.currentState_s = s_cLevel
}

func (sch *scheduler) GetStateAndRewardFQSATLost(s *session, pth *path) {
	lRTT := make(map[protocol.PathID]time.Duration)
	sRTT := make(map[protocol.PathID]time.Duration)

	cwnd := make(map[protocol.PathID]protocol.ByteCount)
	cwndlevel := make(map[protocol.PathID]float32)
	inp := make(map[protocol.PathID]protocol.ByteCount)

	reWard := make(map[protocol.PathID]float64)

	firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)

	for pathID, path := range s.paths {
		//dataString := fmt.Sprintf("%d -", int(pathID))
		//f.WriteString(dataString)
		if pathID != protocol.InitialPathID {
			//packetNumber[pathID], retransNumber[pathID], _ = path.sentPacketHandler.GetStatistics()
			lRTT[pathID] = path.rttStats.LatestRTT()
			sRTT[pathID] = path.rttStats.SmoothedRTT()
			cwnd[pathID] = path.sentPacketHandler.GetCongestionWindow()
			inp[pathID] = path.sentPacketHandler.GetBytesInFlight()
			if float32(cwnd[pathID]) != 0 {
				cwndlevel[pathID] = float32(path.sentPacketHandler.GetBytesInFlight()) / float32(cwnd[pathID])
			} else {
				cwndlevel[pathID] = 0
			}

			// Ordering paths
			if firstPath == protocol.PathID(255) {
				firstPath = pathID
			} else {
				if pathID < firstPath {
					secondPath = firstPath
					firstPath = pathID
				} else {
					secondPath = pathID
				}
			}
		}
	}
	reWard[firstPath] = -s.scheduler.Beta
	reWard[secondPath] = -s.scheduler.Beta

	f_sendingRate := (float64(cwnd[firstPath]) / float64(lRTT[firstPath])) / (float64(cwnd[firstPath])/float64(lRTT[firstPath]) + float64(cwnd[secondPath])/float64(lRTT[secondPath]))
	s_sendingRate := (float64(cwnd[secondPath]) / float64(lRTT[secondPath])) / (float64(cwnd[firstPath])/float64(lRTT[firstPath]) + float64(cwnd[secondPath])/float64(lRTT[secondPath]))

	//update Q
	var f_cLevel, s_cLevel, col int8

	if pth.pathID == firstPath {
		col = 0
		if reWard[firstPath] == 0 {
			return
		}
	} else {
		if reWard[secondPath] == 0 {
			return
		}
		col = 1
	}

	if f_sendingRate < sch.clv_state[0] {
		f_cLevel = 0
	} else if f_sendingRate < sch.clv_state[1] {
		f_cLevel = 1
	} else if f_sendingRate < sch.clv_state[2] {
		f_cLevel = 2
	} else if f_sendingRate < sch.clv_state[3] {
		f_cLevel = 3
	} else {
		f_cLevel = 4
	}

	if s_sendingRate < sch.clv_state[0] {
		s_cLevel = 0
	} else if s_sendingRate < sch.clv_state[1] {
		s_cLevel = 1
	} else if s_sendingRate < sch.clv_state[2] {
		s_cLevel = 2
	} else if s_sendingRate < sch.clv_state[3] {
		s_cLevel = 3
	} else {
		s_cLevel = 4
	}

	old_f_cLevel := s.scheduler.currentState_f
	old_s_cLevel := s.scheduler.currentState_s

	var maxNextState float64
	if s.scheduler.qtable[f_cLevel][s_cLevel][0] > s.scheduler.qtable[f_cLevel][s_cLevel][1] {
		maxNextState = s.scheduler.qtable[f_cLevel][s_cLevel][0]
	} else {
		maxNextState = s.scheduler.qtable[f_cLevel][s_cLevel][1]
	}
	// fmt.Println("Reward2")
	newValue := (1-s.scheduler.Delta)*s.scheduler.qtable[old_f_cLevel][old_s_cLevel][col] + (s.scheduler.Delta)*(reWard[pth.pathID]+s.scheduler.Gamma*maxNextState)

	s.scheduler.qtable[old_f_cLevel][old_s_cLevel][col] = newValue
	s.scheduler.currentState_f = f_cLevel
	s.scheduler.currentState_s = s_cLevel
}

func (sch *scheduler) GetStateAndRewardQSAT(s *session, pth *path) {
	rcvdpacketNumber := pth.lastRcvdPacketNumber
	packetNumber := make(map[protocol.PathID]uint64)
	retransNumber := make(map[protocol.PathID]uint64)

	sRTT := make(map[protocol.PathID]time.Duration)
	maxRTT := make(map[protocol.PathID]time.Duration)

	cwnd := make(map[protocol.PathID]protocol.ByteCount)
	cwndlevel := make(map[protocol.PathID]float32)

	reWard := make(map[protocol.PathID]float64)

	firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)

	for pathID, path := range s.paths {
		//dataString := fmt.Sprintf("%d -", int(pathID))
		//f.WriteString(dataString)
		if pathID != protocol.InitialPathID {
			packetNumber[pathID], retransNumber[pathID], _ = path.sentPacketHandler.GetStatistics()
			sRTT[pathID] = path.rttStats.LatestRTT()
			maxRTT[pathID] = path.rttStats.MaxRTT()
			cwnd[pathID] = path.sentPacketHandler.GetCongestionWindow()
			if float32(cwnd[pathID]) != 0 {
				cwndlevel[pathID] = float32(path.sentPacketHandler.GetBytesInFlight()) / float32(cwnd[pathID])
			} else {
				cwndlevel[pathID] = 1
			}

			// Ordering paths
			if firstPath == protocol.PathID(255) {
				firstPath = pathID
			} else {
				if pathID < firstPath {
					secondPath = firstPath
					firstPath = pathID
				} else {
					secondPath = pathID
				}
			}
		}
	}

	if maxRTT[firstPath] <= maxRTT[secondPath] {
		maxRTT[firstPath] = maxRTT[secondPath]
	} else {
		maxRTT[secondPath] = maxRTT[firstPath]
	}

	// goodput1 := NormalizeGoodput(s, packetNumber[firstPath], retransNumber[firstPath])
	// goodput2 := NormalizeGoodput(s, packetNumber[secondPath], retransNumber[secondPath])

	//fmt.Println(goodput1, goodput2)

	//fmt.Println(NormalizeTimes(sRTT[firstPath]), NormalizeTimes(maxRTT[firstPath]), NormalizeTimes(sRTT[secondPath]), NormalizeTimes(maxRTT[secondPath]))

	if float32(packetNumber[firstPath]) > 0 {
		reWard[firstPath] = (1-s.scheduler.Alpha-s.scheduler.Beta)*float64(cwndlevel[firstPath]) - s.scheduler.Alpha*float64(NormalizeTimes(sRTT[firstPath])/50) - s.scheduler.Beta*float64(5*float32(retransNumber[firstPath])/float32(packetNumber[firstPath]))
	} else {
		reWard[firstPath] = (1-s.scheduler.Alpha-s.scheduler.Beta)*float64(cwndlevel[firstPath]) - s.scheduler.Alpha*float64(NormalizeTimes(sRTT[firstPath])/50)
	}
	if float32(packetNumber[secondPath]) > 0 {
		reWard[secondPath] = (1-s.scheduler.Alpha-s.scheduler.Beta)*float64(cwndlevel[secondPath]) - s.scheduler.Alpha*float64(NormalizeTimes(sRTT[secondPath])/50) - s.scheduler.Beta*float64(5*float32(retransNumber[secondPath])/float32(packetNumber[secondPath]))
	} else {
		reWard[secondPath] = (1-s.scheduler.Alpha-s.scheduler.Beta)*float64(cwndlevel[secondPath]) - s.scheduler.Alpha*float64(NormalizeTimes(sRTT[secondPath])/50)
	}

	// f, err := os.OpenFile("/App/output/factor.data", os.O_APPEND|os.O_WRONLY, 0644)
	// if err != nil {
	// 	panic(err)
	// }
	// defer f.Close()
	// dataString6 := fmt.Sprintf("%d - %d - %d\n",pth.pathID, firstPath, secondPath)
	// dataString8 := fmt.Sprintf("%f - %f\n",s.scheduler.Alpha, s.scheduler.Beta)
	// dataString := fmt.Sprintf("%f - %f - %f\n",reWard[pth.pathID], reWard[firstPath], reWard[secondPath] )
	// dataString3 := fmt.Sprintf("%f - %f - %f\n---\n", float64(cwndlevel[pth.pathID]) , float64(NormalizeTimes(sRTT[pth.pathID])), float64(5*float32(retransNumber[pth.pathID])/float32(packetNumber[pth.pathID])))
	// f.WriteString(dataString6)
	// f.WriteString(dataString8)
	// f.WriteString(dataString)
	// f.WriteString(dataString3)

	//State
	oldBSend := s.scheduler.QoldState[State{id: pth.pathID, pktnumber: rcvdpacketNumber}]
	delete(s.scheduler.QoldState, State{id: pth.pathID, pktnumber: rcvdpacketNumber})

	var BSend protocol.ByteCount
	var BSend1 float32
	// var numStream uint8
	// if s.streamsMap != nil {
	// 	if s.streamsMap.perspective == 1 && s.streamsMap.streams != nil {
	// 		for streamID, _ := range s.streamsMap.streams {
	// 			if streamID > 3 {
	// 				tmpBSend, _ := s.flowControlManager.SendWindowSize(streamID)
	// 				BSend += tmpBSend
	// 				numStream += 1
	// 				// fmt.Printf("Stream ID: %d, Stream Property: %v\n", streamID, BSend)
	// 			}
	// 		}
	// 	}
	// }
	// if numStream != 0 {
	// 	BSend1 = float32(BSend) / (float32(protocol.DefaultMaxCongestionWindow) * 300 * float32(numStream) * 5)
	// } else {
	// 	BSend, _ = s.flowControlManager.SendWindowSize(protocol.StreamID(5))
	// 	BSend1 = float32(BSend) / (float32(protocol.DefaultMaxCongestionWindow) * 300)
	// }
	BSend, _ = s.flowControlManager.SendWindowSize(protocol.StreamID(5))
	BSend1 = float32(BSend) / (float32(protocol.DefaultMaxCongestionWindow) * 300)
	var ro, col, ro1 int8

	if pth.pathID == firstPath {
		col = 0
		if reWard[firstPath] == 0 {
			return
		}
	} else {
		if reWard[secondPath] == 0 {
			return
		}
		col = 1
	}

	if float64(BSend1) < sch.Qstate[0] {
		ro = 0
	} else if float64(BSend1) < sch.Qstate[1] {
		ro = 1
	} else if float64(BSend1) < sch.Qstate[2] {
		ro = 2
	} else if float64(BSend1) < sch.Qstate[3] {
		ro = 3
	} else if float64(BSend1) < sch.Qstate[4] {
		ro = 4
	} else if float64(BSend1) < sch.Qstate[5] {
		ro = 5
	} else if float64(BSend1) < sch.Qstate[6] {
		ro = 6
	} else {
		ro = 7
	}

	if float64(oldBSend) < sch.Qstate[0] {
		ro1 = 0
	} else if float64(oldBSend) < sch.Qstate[1] {
		ro1 = 1
	} else if float64(oldBSend) < sch.Qstate[2] {
		ro1 = 2
	} else if float64(oldBSend) < sch.Qstate[3] {
		ro1 = 3
	} else if float64(oldBSend) < sch.Qstate[4] {
		ro1 = 4
	} else if float64(oldBSend) < sch.Qstate[5] {
		ro1 = 5
	} else if float64(oldBSend) < sch.Qstate[6] {
		ro1 = 6
	} else {
		ro1 = 7
	}

	var maxNextState float64
	if s.scheduler.Qqtable[Store{Row: ro, Col: 1}] > s.scheduler.Qqtable[Store{Row: ro, Col: 0}] {
		maxNextState = s.scheduler.Qqtable[Store{Row: ro, Col: 1}]
	} else {
		maxNextState = s.scheduler.Qqtable[Store{Row: ro, Col: 0}]
	}

	newValue := (1-s.scheduler.Delta)*s.scheduler.Qqtable[Store{Row: ro1, Col: col}] + s.scheduler.Delta*(reWard[pth.pathID]+s.scheduler.Gamma*maxNextState)

	s.scheduler.Qqtable[Store{Row: ro1, Col: col}] = newValue

	//fmt.Println(ro1, col, reWard[pth.pathID])
	//fmt.Println(float64(cwndlevel[pth.pathID]), float64(NormalizeTimes(sRTT[pth.pathID]) / NormalizeTimes(maxRTT[pth.pathID])), float64(10*float32(retransNumber[pth.pathID])/float32(packetNumber[pth.pathID])))
	//fmt.Println(s.scheduler.qtable[store{row: ro1, col: col}])
	//fmt.Println("-----------")

	// if len(s.scheduler.qtable) > 0{
	// 	fmt.Println("Okkkkkkkkkkkkkkkkkk")
	// 	fmt.Println(s.scheduler.qtable)
	// }else{
	// 	fmt.Println("Errrrrrrrrrrrrrrrrr")
	// }
	//fmt.Println(s.scheduler.qtable[ro1][col], newValue, "Update 2")

	// dataString2 := fmt.Sprintf("%f - %d - %d - %f - %f - %f\n", float32(BSend), ro1, col, float64(s.scheduler.qtable[ro1][col]), float64(reWard[pth.pathID]), maxNextState)
	// dataString4 := fmt.Sprintf("%f - %f - %f\n---\n", s.scheduler.Delta, s.scheduler.Gamma, pth.sess.scheduler.Gamma)
	// f2, err2 := os.OpenFile("/App/output/qtable.data", os.O_APPEND|os.O_WRONLY, 0644)
	// if err2 != nil {
	// 	panic(err2)
	// }
	// defer f2.Close()

	// f2.WriteString(dataString2)
	// f2.WriteString(dataString4)

	// dataString5 := fmt.Sprintf("%f - %f - %d\n", BSend1, oldBSend, rcvdpacketNumber)
	// f3, err3 := os.OpenFile("/App/output/bsent.data", os.O_APPEND|os.O_WRONLY, 0644)
	// if err3 != nil {
	// 	panic(err3)
	// }
	// defer f3.Close()

	// f3.WriteString(dataString5)
}

func updateReward(url string, payload RewardPayload) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Use a goroutine to perform the POST request without waiting for the response
	go func() {
		_, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			fmt.Println("Error sending POST request:", err)
		}
	}()

	return nil
}

func (sch *scheduler) GetStateAndRewardDQN(s *session, pth *path) {
	packetNumber := make(map[protocol.PathID]uint64)
	retransNumber := make(map[protocol.PathID]uint64)
	lostNumber := make(map[protocol.PathID]uint64)

	lRTT := make(map[protocol.PathID]time.Duration)
	sRTT := make(map[protocol.PathID]time.Duration)
	mRTT := make(map[protocol.PathID]time.Duration)

	CWND := make(map[protocol.PathID]protocol.ByteCount)
	CWNDlevel := make(map[protocol.PathID]float32)
	INP := make(map[protocol.PathID]protocol.ByteCount)

	reWard := make(map[protocol.PathID]float64)

	firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)

	for pathID, path := range s.paths {
		if pathID != protocol.InitialPathID {
			packetNumber[pathID], retransNumber[pathID], lostNumber[pathID] = path.sentPacketHandler.GetStatistics()
			lRTT[pathID] = path.rttStats.LatestRTT()
			sRTT[pathID] = path.rttStats.SmoothedRTT()
			mRTT[pathID] = path.rttStats.MinRTT()
			CWND[pathID] = path.sentPacketHandler.GetCongestionWindow()
			INP[pathID] = path.sentPacketHandler.GetBytesInFlight()
			if float32(CWND[pathID]) != 0 {
				CWNDlevel[pathID] = float32(path.sentPacketHandler.GetBytesInFlight()) / float32(CWND[pathID])
			} else {
				CWNDlevel[pathID] = 1
			}
			// Ordering paths
			if firstPath == protocol.PathID(255) {
				firstPath = pathID
			} else {
				if pathID < firstPath {
					secondPath = firstPath
					firstPath = pathID
				} else {
					secondPath = pathID
				}
			}
		}
	}

	//alpha := 0.01
	//reWard[firstPath] = goodput1 - alpha*float64(NormalizeTimes(lRTT[firstPath])) - float64(lostNumber[firstPath]/packetNumber[firstPath])
	//reWard[secondPath] = goodput1 - alpha*float64(NormalizeTimes(lRTT[secondPath]))- float64(lostNumber[secondPath]/packetNumber[secondPath])

	rttrate := 0.0
	goodput := NormalizeGoodput(s, packetNumber[pth.pathID], retransNumber[pth.pathID])
	lostrate := 10 * float64(lostNumber[pth.pathID]) / float64(packetNumber[pth.pathID])
	if mRTT[pth.pathID] != 0 {
		rttrate = float64(lRTT[pth.pathID]) / float64(mRTT[pth.pathID])
	}

	reWard[pth.pathID] = float64(goodput) - rttrate - lostrate
	// fmt.Println("reWard", float64(goodput), rttrate, lostrate)
	old_state := sch.list_State_DQN[State{pth.pathID, pth.lastRcvdPacketNumber}]
	old_action := sch.list_Action_DQN[State{pth.pathID, pth.lastRcvdPacketNumber}]

	nextState := StateDQN{
		CWNDf: float64(CWND[firstPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
		INPf:  float64(INP[firstPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
		SRTTf: NormalizeTimes(sRTT[firstPath]) / 50.0,
		CWNDs: float64(CWND[secondPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
		INPs:  float64(INP[secondPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
		SRTTs: NormalizeTimes(sRTT[secondPath]) / 50.0,
	}

	rewardPayload := RewardPayload{
		State:     old_state,
		NextState: nextState,
		Action:    old_action,
		Reward:    reWard[pth.pathID],
		Done:      false,
	}
	// fmt.Println("PayLoad: ", rewardPayload)
	err := updateReward(baseURL+"/update_reward", rewardPayload)
	if err != nil {
		fmt.Println("Error updating reward:", err)
		return
	}
}

func (sch *scheduler) GetStateAndRewardSAC(s *session, pth *path) {
	packetNumber := make(map[protocol.PathID]uint64)
	retransNumber := make(map[protocol.PathID]uint64)
	lostNumber := make(map[protocol.PathID]uint64)

	lRTT := make(map[protocol.PathID]time.Duration)
	sRTT := make(map[protocol.PathID]time.Duration)
	mRTT := make(map[protocol.PathID]time.Duration)

	CWND := make(map[protocol.PathID]protocol.ByteCount)
	CWNDlevel := make(map[protocol.PathID]float32)
	INP := make(map[protocol.PathID]protocol.ByteCount)

	reWard := make(map[protocol.PathID]float64)

	for pathID, path := range s.paths {
		if pathID != protocol.InitialPathID {
			packetNumber[pathID], retransNumber[pathID], lostNumber[pathID] = path.sentPacketHandler.GetStatistics()
			lRTT[pathID] = path.rttStats.LatestRTT()
			sRTT[pathID] = path.rttStats.SmoothedRTT()
			mRTT[pathID] = path.rttStats.MinRTT()
			CWND[pathID] = path.sentPacketHandler.GetCongestionWindow()
			INP[pathID] = path.sentPacketHandler.GetBytesInFlight()
			if float32(CWND[pathID]) != 0 {
				CWNDlevel[pathID] = float32(path.sentPacketHandler.GetBytesInFlight()) / float32(CWND[pathID])
			} else {
				CWNDlevel[pathID] = 1
			}
		}
	}

	rttrate := 0.0
	goodput := NormalizeGoodput(s, packetNumber[pth.pathID], retransNumber[pth.pathID])
	// lostrate := float64(retransNumber[pth.pathID]) / float64(packetNumber[pth.pathID])
	rttrate = NormalizeTimes(lRTT[pth.pathID])
	// restranrate := float64(retransNumber[pth.pathID]) / float64(packetNumber[pth.pathID])

	reWard[pth.pathID] = sch.Beta*NormalizeRewardGoodput(goodput) - (1-sch.Beta)*NormalizeRewardLrtt(rttrate)
	// fmt.Println("Reward: ", reWard[pth.pathID], NormalizeRewardGoodput(goodput), NormalizeRewardLrtt(rttrate), sch.Alpha, sch.Beta)

	old_action := sch.list_Action_DQN[State{pth.pathID, pth.lastRcvdPacketNumber}]

	var connID string = strconv.FormatFloat(old_action, 'E', -1, 64)
	if tmp_Reload, ok := multiclients.List_Reward_DQN.Get(connID); ok {
		// fmt.Println("State: ", stateData, rewardPayload)
		rewardPayload, ok := tmp_Reload.(RewardPayload)
		if !ok {
			fmt.Println("Type assertion failed")
			return
		}
		if pth.pathID == 1 {
			rewardPayload.Reward += reWard[pth.pathID] * old_action
		} else {
			rewardPayload.Reward += reWard[pth.pathID] * (1 - old_action)
		}
		rewardPayload.CountReward += 1
		k := uint16(0)
		if uint16(sch.Alpha) < 6 {
			k = uint16(sch.Alpha)
		} else {
			k = uint16(sch.Alpha) - 5
		}
		if rewardPayload.CountReward >= k {
			rewardPayload.Reward = rewardPayload.Reward / float64(rewardPayload.CountReward)
			rewardPayload.Action = old_action

			tmp_Payload := RewardPayload{
				State:     rewardPayload.State,
				NextState: rewardPayload.NextState,
				Action:    rewardPayload.Action,
				Reward:    rewardPayload.Reward,
				Done:      false,
				ModelID:   sch.model_id,
			}

			// fmt.Println("Reward: ", tmp_Payload)
			updateReward(baseURL+"/update_reward", tmp_Payload)

			// multiclients.List_Reward_DQN.Remove(connID)
			rewardPayload.Reward = 0
			rewardPayload.CountReward = 0
			multiclients.List_Reward_DQN.Set(connID, rewardPayload)
		} else {
			multiclients.List_Reward_DQN.Set(connID, rewardPayload)
		}
	}
}
