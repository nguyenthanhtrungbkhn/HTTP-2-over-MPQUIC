package quic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/lucas-clemente/quic-go/ackhandler"
	"github.com/lucas-clemente/quic-go/internal/multiclients"
	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
	"github.com/lucas-clemente/quic-go/internal/wire"
	"gonum.org/v1/gonum/mat"
)

const beta uint64 = 4

type Store struct {
	Row int8
	Col int8
}
type scheduler struct {
	quotas        map[protocol.PathID]uint
	pathScheduler func(*session, bool, bool, *path) *path
	SchedulerName string
	waiting       uint64

	//Peekaboo
	MAaF [banditDimension][banditDimension]float64
	MAaS [banditDimension][banditDimension]float64
	MbaF [banditDimension]float64
	MbaS [banditDimension]float64

	// Qlearning
	QminAlpha   float64
	Qqtable     map[Store]float64
	QoldState   map[State]float64
	Qstate      [11]float64
	QcountState [11]uint32
	QoldQ       float64

	//FQSAT
	qtable         [5][5][2]float64
	clv_state      [4]float64
	currentState_f int8
	currentState_s int8

	oldState   map[State]float64
	state      [11]float64
	countState [5][5]uint32
	f_cTable   map[State]int8
	s_cTable   map[State]int8
	iRTT       map[protocol.PathID]float64

	record     uint64
	Epsilon    float64
	Alpha      float64
	Beta       float64
	Delta      float64
	Gamma      float64
	AdaDivisor float64

	//DQN
	current_Action protocol.PathID
	// current_StreamID  uint32
	// flag_train        bool
	//SAC
	list_State_DQN    map[State]StateDQN
	list_Action_DQN   map[State]float64
	current_State_DQN StateDQN
	current_Prob      float64
	current_Reward    float64
	count_Reward      uint16
	model_id          uint64
	time_Get_Action   time.Time
	list_Reward_DQN   map[float64]RewardPayload

	//SACMulti
	list_State_SACMulti    map[State]StateSACMulti
	list_Action_SACMulti   map[State]float64
	list_Reward_SACMulti   map[float64]RewardPayloadSACMulti
	current_State_SACMulti StateSACMulti
	count_Action           uint16

	//SACMultiJoinCC
	list_Action_SACMultiJoinCC map[State]ActionJoinCC
	list_Reward_SACMultiJoinCC map[float64]RewardPayloadSACMultiJoinCC
	current_Prob_JoinCC        ActionJoinCC
}

func (sch *scheduler) setup(pathScheduler string) {
	sch.quotas = make(map[protocol.PathID]uint)
	sch.iRTT = make(map[protocol.PathID]float64)
	sch.waiting = 0

	if pathScheduler == "QSAT" {
		sch.QoldState = make(map[State]float64)
		sch.Qqtable = make(map[Store]float64)
		var config [6]float64
		f, err := os.Open("./config/qsat")
		if err != nil {
			panic(err)
		}

		for i := 0; i < 6; i++ {
			fmt.Fscanln(f, &config[i])
		}
		sch.Alpha = config[0]
		sch.Beta = config[1]
		sch.Delta = config[2]
		sch.Gamma = config[3]
		sch.Epsilon = config[4]
		sch.AdaDivisor = config[5]
		f.Close()

		sch.Qstate[0] = 0.05
		sch.Qstate[1] = 0.10
		sch.Qstate[2] = 0.15
		sch.Qstate[3] = 0.20
		sch.Qstate[4] = 0.30
		sch.Qstate[5] = 0.40
		sch.Qstate[6] = 0.60

		fmt.Println(config)
		sch.record = 1
	}

	if pathScheduler == "Peekaboo" {
		//Read lin to buffer
		file, err := os.Open("./config/lin")
		if err != nil {
			panic(err)
		}

		for i := 0; i < banditDimension; i++ {
			for j := 0; j < banditDimension; j++ {
				fmt.Fscanln(file, &sch.MAaF[i][j])
			}
		}
		for i := 0; i < banditDimension; i++ {
			for j := 0; j < banditDimension; j++ {
				fmt.Fscanln(file, &sch.MAaS[i][j])
			}
		}
		for i := 0; i < banditDimension; i++ {
			fmt.Fscanln(file, &sch.MbaF[i])
		}
		for i := 0; i < banditDimension; i++ {
			fmt.Fscanln(file, &sch.MbaS[i])
		}
		file.Close()
	}

	if pathScheduler == "FQSAT" {
		sch.oldState = make(map[State]float64)
		sch.f_cTable = make(map[State]int8)
		sch.s_cTable = make(map[State]int8)
		var config [6]float64
		f, err := os.Open("./config/fqsat")
		if err != nil {
			panic(err)
		}

		for i := 0; i < 6; i++ {
			fmt.Fscanln(f, &config[i])
		}
		sch.Alpha = config[0]
		sch.Beta = config[1]
		sch.Delta = config[2]
		sch.Gamma = config[3]
		sch.Epsilon = config[4]
		sch.AdaDivisor = config[5]
		f.Close()

		sch.state[0] = 0.05
		sch.state[1] = 0.10
		sch.state[2] = 0.15
		sch.state[3] = 0.20
		sch.state[4] = 0.30
		sch.state[5] = 0.40
		sch.state[6] = 0.60

		sch.clv_state[0] = 0.3
		sch.clv_state[1] = 0.4
		sch.clv_state[2] = 0.5
		sch.clv_state[3] = 0.6
		fmt.Println(config)
		sch.record = 1
	}

	if pathScheduler == "DQN" {
		sch.list_State_DQN = make(map[State]StateDQN)
		// modelType := map[string]string{"model_type": "dqn"}
		// setModel(modelType)
	} else if pathScheduler == "SAC" {
		f, err := os.Open("./config/sac")
		if err != nil {
			panic(err)
		}
		var config [2]float64
		for i := 0; i < 2; i++ {
			fmt.Fscanln(f, &config[i])
		}
		sch.Alpha = config[0]
		sch.Beta = config[1]

		sch.list_State_DQN = make(map[State]StateDQN)
		sch.list_Action_DQN = make(map[State]float64)
		sch.model_id = TMP_ModelID
		// fmt.Println("Check_K: ", sch.Alpha)
		sch.count_Action = 0
		// modelType := map[string]string{"model_type": "sac", "model_id": string(sch.model_id)}
		setModel("sac", sch.model_id)
	} else if sch.SchedulerName == "sacmulti" || sch.SchedulerName == "sacrx" {
		sch.list_State_SACMulti = make(map[State]StateSACMulti)
		sch.list_Action_SACMulti = make(map[State]float64)
		sch.list_Reward_SACMulti = make(map[float64]RewardPayloadSACMulti)

		sch.count_Action = 0
		// sch.model_id = TMP_ModelID
		// fmt.Println("CheckllL: ", TMP_ModelID)
		// modelType := map[string]string{"model_type": "sac", "model_id": string(sch.model_id)}
		// setModel("sac", sch.model_id)
		SV_Txbitrate_interface0 = 7
		SV_Txbitrate_interface1 = 7
	} else if sch.SchedulerName == "sacmultiJoinCC" {
		sch.list_State_SACMulti = make(map[State]StateSACMulti)
		sch.list_Action_SACMultiJoinCC = make(map[State]ActionJoinCC)
		sch.list_Reward_SACMultiJoinCC = make(map[float64]RewardPayloadSACMultiJoinCC)
		sch.count_Action = 0
	}
	//log
	sch.model_id = TMP_ModelID

	if pathScheduler == "RoundRobin" {
		sch.SchedulerName = "RoundRobin"
		sch.pathScheduler = sch.selectPathRoundRobin
	} else if pathScheduler == "ECF" {
		sch.SchedulerName = "ECF"
		sch.pathScheduler = sch.selectPathECF
	} else if pathScheduler == "SAECF" {
		sch.SchedulerName = "SAECF"
		sch.pathScheduler = sch.selectPathStreamAwareEarliestCompletionFirst
	} else if pathScheduler == "QSAT" {
		sch.SchedulerName = "QSAT"
		sch.pathScheduler = sch.selectPathQSAT
	} else if pathScheduler == "FQSAT" {
		sch.SchedulerName = "FQSAT"
		sch.pathScheduler = sch.selectPathFQSAT
	} else if pathScheduler == "BLEST" {
		sch.SchedulerName = "BLEST"
		sch.pathScheduler = sch.selectPathBLEST
	} else if pathScheduler == "Peekaboo" {
		sch.SchedulerName = "Peekaboo"
		sch.pathScheduler = sch.selectPathPeekaboo
	} else if pathScheduler == "SAC" {
		sch.SchedulerName = "SAC"
		sch.pathScheduler = sch.selectPathSAC
	} else {
		sch.SchedulerName = "LowLatency"
		sch.pathScheduler = sch.selectPathLowLatency
	}
}

func (sch *scheduler) getRetransmission(s *session) (hasRetransmission bool, retransmitPacket *ackhandler.Packet, pth *path) {
	// check for retransmissions first
	for {
		// TODO add ability to reinject on another path
		// XXX We need to check on ALL paths if any packet should be first retransmitted
		s.pathsLock.RLock()
	retransmitLoop:
		for _, pthTmp := range s.paths {
			retransmitPacket = pthTmp.sentPacketHandler.DequeuePacketForRetransmission()
			if retransmitPacket != nil {
				pth = pthTmp
				break retransmitLoop
			}
		}
		s.pathsLock.RUnlock()
		if retransmitPacket == nil {
			break
		}
		hasRetransmission = true

		if retransmitPacket.EncryptionLevel != protocol.EncryptionForwardSecure {
			if s.handshakeComplete {
				// Don't retransmit handshake packets when the handshake is complete
				continue
			}
			utils.Debugf("\tDequeueing handshake retransmission for packet 0x%x", retransmitPacket.PacketNumber)
			return
		}
		utils.Debugf("\tDequeueing retransmission of packet 0x%x from path %d", retransmitPacket.PacketNumber, pth.pathID)
		// resend the frames that were in the packet
		for _, frame := range retransmitPacket.GetFramesForRetransmission() {
			switch f := frame.(type) {
			case *wire.StreamFrame:
				s.streamFramer.AddFrameForRetransmission(f)
			case *wire.WindowUpdateFrame:
				// only retransmit WindowUpdates if the stream is not yet closed and the we haven't sent another WindowUpdate with a higher ByteOffset for the stream
				// XXX Should it be adapted to multiple paths?
				currentOffset, err := s.flowControlManager.GetReceiveWindow(f.StreamID)
				if err == nil && f.ByteOffset >= currentOffset {
					s.packer.QueueControlFrame(f, pth)
				}
			case *wire.PathsFrame:
				// Schedule a new PATHS frame to send
				s.schedulePathsFrame()
			default:
				s.packer.QueueControlFrame(frame, pth)
			}
		}
	}
	return
}

func (sch *scheduler) selectPathRoundRobin(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	/*if sch.quotas == nil {
		sch.setup("RoundRobin", sch.streamScheduler)
	}*/

	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// TODO cope with decreasing number of paths (needed?)
	var selectedPath *path
	var lowerQuota, currentQuota uint
	var ok bool

	// Max possible value for lowerQuota at the beginning
	lowerQuota = ^uint(0)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do no consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentQuota, ok = sch.quotas[pathID]
		if !ok {
			sch.quotas[pathID] = 0
			currentQuota = 0
		}

		if currentQuota < lowerQuota {
			selectedPath = pth
			lowerQuota = currentQuota
		}
	}

	return selectedPath

}

func (sch *scheduler) selectPathLowLatency(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var selectedPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	selectedPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[selectedPathID]
			if selectedPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT != 0 && lowerRTT != 0 && selectedPath != nil && currentRTT >= lowerRTT {
			continue pathLoop
		}

		// Update
		lowerRTT = currentRTT
		selectedPath = pth
		selectedPathID = pathID
	}

	return selectedPath
}

func (sch *scheduler) selectPathBLEST(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var bestPath *path
	var secondBestPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	var secondLowerRTT time.Duration
	bestPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[bestPathID]
			if bestPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT >= lowerRTT {
			if (secondLowerRTT == 0 || currentRTT < secondLowerRTT) && pth.SendingAllowed() {
				// Update second best available path
				secondLowerRTT = currentRTT
				secondBestPath = pth
			}
			if currentRTT != 0 && lowerRTT != 0 && bestPath != nil {
				continue pathLoop
			}
		}

		// Update
		lowerRTT = currentRTT
		bestPath = pth
		bestPathID = pathID
	}

	if bestPath == nil {
		if secondBestPath != nil {
			return secondBestPath
		}
		return nil
	}

	if hasRetransmission || bestPath.SendingAllowed() {
		return bestPath
	}

	if secondBestPath == nil {
		return nil
	}
	cwndBest := uint64(bestPath.sentPacketHandler.GetCongestionWindow())
	FirstCo := uint64(protocol.DefaultTCPMSS) * uint64(secondLowerRTT) * (cwndBest*2*uint64(lowerRTT) + uint64(secondLowerRTT) - uint64(lowerRTT))
	BSend, _ := s.flowControlManager.SendWindowSize(protocol.StreamID(5))
	SecondCo := 2 * 1 * uint64(lowerRTT) * uint64(lowerRTT) * (uint64(BSend) - (uint64(secondBestPath.sentPacketHandler.GetBytesInFlight()) + uint64(protocol.DefaultTCPMSS)))

	if FirstCo > SecondCo {
		return nil
	} else {
		return secondBestPath
	}
}

func (sch *scheduler) selectPathECF(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var bestPath *path
	var secondBestPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	var secondLowerRTT time.Duration
	bestPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[bestPathID]
			if bestPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT >= lowerRTT {
			if (secondLowerRTT == 0 || currentRTT < secondLowerRTT) && pth.SendingAllowed() {
				// Update second best available path
				secondLowerRTT = currentRTT
				secondBestPath = pth
			}
			if currentRTT != 0 && lowerRTT != 0 && bestPath != nil {
				continue pathLoop
			}
		}

		// Update
		lowerRTT = currentRTT
		bestPath = pth
		bestPathID = pathID
	}

	if bestPath == nil {
		if secondBestPath != nil {
			return secondBestPath
		}
		return nil
	}

	if hasRetransmission || bestPath.SendingAllowed() {
		return bestPath
	}

	if secondBestPath == nil {
		return nil
	}

	var queueSize uint64
	getQueueSize := func(s *stream) (bool, error) {
		if s != nil {
			queueSize = queueSize + uint64(s.lenOfDataForWriting())
		}
		return true, nil
	}
	s.streamsMap.Iterate(getQueueSize)

	cwndBest := uint64(bestPath.sentPacketHandler.GetCongestionWindow())
	cwndSecond := uint64(secondBestPath.sentPacketHandler.GetCongestionWindow())
	deviationBest := uint64(bestPath.rttStats.MeanDeviation())
	deviationSecond := uint64(secondBestPath.rttStats.MeanDeviation())

	delta := deviationBest
	if deviationBest < deviationSecond {
		delta = deviationSecond
	}
	xBest := queueSize
	if queueSize < cwndBest {
		xBest = cwndBest
	}

	lhs := uint64(lowerRTT) * (xBest + cwndBest)
	rhs := cwndBest * (uint64(secondLowerRTT) + delta)
	if (lhs * 4) < ((rhs * 4) + sch.waiting*rhs) {
		xSecond := queueSize
		if queueSize < cwndSecond {
			xSecond = cwndSecond
		}
		lhsSecond := uint64(secondLowerRTT) * xSecond
		rhsSecond := cwndSecond * (2*uint64(lowerRTT) + delta)
		if lhsSecond > rhsSecond {
			sch.waiting = 1
			return nil
		}
	} else {
		sch.waiting = 0
	}

	return secondBestPath
}

func (sch *scheduler) selectPathPeekaboo(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var bestPath *path
	var secondBestPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	var secondLowerRTT time.Duration
	bestPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[bestPathID]
			if bestPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT >= lowerRTT {
			if (secondLowerRTT == 0 || currentRTT < secondLowerRTT) && pth.SendingAllowed() {
				// Update second best available path
				secondLowerRTT = currentRTT
				secondBestPath = pth
			}
			if currentRTT != 0 && lowerRTT != 0 && bestPath != nil {
				continue pathLoop
			}
		}

		// Update
		lowerRTT = currentRTT
		bestPath = pth
		bestPathID = pathID

	}

	//Paths
	// firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)
	// for pathID, _ := range s.paths{
	// 	if pathID != protocol.InitialPathID{
	// 		// fmt.Println("PathID: ", pathID)
	// 		if firstPath == protocol.PathID(255){
	// 			firstPath = pathID
	// 		}else{
	// 			if pathID < firstPath{
	// 				secondPath = firstPath
	// 				firstPath = pathID
	// 			}else{
	// 				secondPath = pathID
	// 			}
	// 		}
	// 	}

	// }
	// if secondPath != protocol.PathID(255) && firstPath != protocol.PathID(255) {
	// 	dataString2 := fmt.Sprintf("%d,%d,%d,%d,%d,%d\n",s.paths[protocol.InitialPathID].sentPacketHandler.GetBytesInFlight(),
	// 																s.paths[firstPath].sentPacketHandler.GetBytesInFlight(),
	// 																s.paths[secondPath].sentPacketHandler.GetBytesInFlight(),
	// 																s.paths[protocol.InitialPathID].sentPacketHandler.GetCongestionWindow(),
	// 																s.paths[firstPath].sentPacketHandler.GetCongestionWindow(),
	// 																s.paths[secondPath].sentPacketHandler.GetCongestionWindow())
	// 	f2, err2 := os.OpenFile("/App/output/state_action.csv", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	// 	if err2 != nil {
	// 		panic(err2)
	// 	}
	// 	defer f2.Close()

	// 	f2.WriteString(dataString2)
	// }

	if bestPath == nil {
		if secondBestPath != nil {
			return secondBestPath
		}
		if s.paths[protocol.InitialPathID].SendingAllowed() && hasRetransmission {
			return s.paths[protocol.InitialPathID]
		} else {
			return nil
		}
	}
	if bestPath.SendingAllowed() {
		sch.waiting = 0
		return bestPath
	}
	if secondBestPath == nil {
		if s.paths[protocol.InitialPathID].SendingAllowed() && hasRetransmission {
			return s.paths[protocol.InitialPathID]
		} else {
			return nil
		}
	}

	if hasRetransmission && secondBestPath.SendingAllowed() {
		return secondBestPath
	}
	if hasRetransmission {
		return s.paths[protocol.InitialPathID]
	}

	if sch.waiting == 1 {
		return nil
	} else {
		// Migrate from buffer to local variables
		AaF := mat.NewDense(banditDimension, banditDimension, nil)
		for i := 0; i < banditDimension; i++ {
			for j := 0; j < banditDimension; j++ {
				AaF.Set(i, j, sch.MAaF[i][j])
			}
		}
		AaS := mat.NewDense(banditDimension, banditDimension, nil)
		for i := 0; i < banditDimension; i++ {
			for j := 0; j < banditDimension; j++ {
				AaS.Set(i, j, sch.MAaS[i][j])
			}
		}
		baF := mat.NewDense(banditDimension, 1, nil)
		for i := 0; i < banditDimension; i++ {
			baF.Set(i, 0, sch.MbaF[i])
		}
		baS := mat.NewDense(banditDimension, 1, nil)
		for i := 0; i < banditDimension; i++ {
			baS.Set(i, 0, sch.MbaS[i])
		}

		//Features
		cwndBest := float64(bestPath.sentPacketHandler.GetCongestionWindow())
		cwndSecond := float64(secondBestPath.sentPacketHandler.GetCongestionWindow())
		BSend, _ := s.flowControlManager.SendWindowSize(protocol.StreamID(5))
		inflightf := float64(bestPath.sentPacketHandler.GetBytesInFlight())
		inflights := float64(secondBestPath.sentPacketHandler.GetBytesInFlight())
		llowerRTT := bestPath.rttStats.LatestRTT()
		lsecondLowerRTT := secondBestPath.rttStats.LatestRTT()
		feature := mat.NewDense(banditDimension, 1, nil)
		if 0 < float64(lsecondLowerRTT) && 0 < float64(llowerRTT) {
			feature.Set(0, 0, cwndBest/float64(llowerRTT))
			feature.Set(2, 0, float64(BSend)/float64(llowerRTT))
			feature.Set(4, 0, inflightf/float64(llowerRTT))
			feature.Set(1, 0, inflights/float64(lsecondLowerRTT))
			feature.Set(3, 0, float64(BSend)/float64(lsecondLowerRTT))
			feature.Set(5, 0, cwndSecond/float64(lsecondLowerRTT))
		} else {
			feature.Set(0, 0, 0)
			feature.Set(2, 0, 0)
			feature.Set(4, 0, 0)
			feature.Set(1, 0, 0)
			feature.Set(3, 0, 0)
			feature.Set(5, 0, 0)
		}

		//Obtain theta
		AaIF := mat.NewDense(banditDimension, banditDimension, nil)
		AaIF.Inverse(AaF)
		thetaF := mat.NewDense(banditDimension, 1, nil)
		thetaF.Product(AaIF, baF)

		AaIS := mat.NewDense(banditDimension, banditDimension, nil)
		AaIS.Inverse(AaS)
		thetaS := mat.NewDense(banditDimension, 1, nil)
		thetaS.Product(AaIS, baS)

		//Obtain bandit value
		thetaFPro := mat.NewDense(1, 1, nil)
		thetaFPro.Product(thetaF.T(), feature)

		thetaSPro := mat.NewDense(1, 1, nil)
		thetaSPro.Product(thetaS.T(), feature)

		//Make decision based on bandit value and stochastic value
		if thetaSPro.At(0, 0) < thetaFPro.At(0, 0) {
			if rand.Intn(100) < 70 {
				sch.waiting = 1
				return nil
			} else {
				sch.waiting = 0
				return secondBestPath
			}
		} else {
			if rand.Intn(100) < 90 {
				sch.waiting = 0
				return secondBestPath
			} else {
				sch.waiting = 1
				return nil
			}
		}
	}

}

func (sch *scheduler) selectPathQSAT(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {

	//return s.paths[protocol.InitialPathID]

	if rand.Float64() > sch.Epsilon {
		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	}

	if len(s.paths) <= 1 {
		//fmt.Println("len1")
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	if len(s.paths) == 2 {
		//fmt.Println("len2")
		for pathID, path := range s.paths {
			if pathID != protocol.InitialPathID {
				utils.Debugf("Selecting path %d as unique path", pathID)
				return path
			}
		}
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	//Paths
	var availablePaths []protocol.PathID
	for pathID := range s.paths {
		if pathID != protocol.InitialPathID {
			availablePaths = append(availablePaths, pathID)
		}
	}

	//fmt.Println(len(availablePaths))
	// if len(availablePaths) == 0{
	// 	if s.paths[protocol.InitialPathID].SendingAllowed() || hasRetransmission{
	// 		return s.paths[protocol.InitialPathID]
	// 	}else{
	// 		return nil
	// 	}
	// }else if len(availablePaths) == 1{
	// 	return s.paths[availablePaths[0]]
	// }

	var ro, action, action2 int8
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
	if float64(BSend1) < sch.state[0] {
		ro = 0
	} else if float64(BSend1) < sch.state[1] {
		ro = 1
	} else if float64(BSend1) < sch.state[2] {
		ro = 2
	} else if float64(BSend1) < sch.state[3] {
		ro = 3
	} else if float64(BSend1) < sch.state[4] {
		ro = 4
	} else if float64(BSend1) < sch.state[5] {
		ro = 5
	} else if float64(BSend1) < sch.state[6] {
		ro = 6
	} else {
		ro = 7
	}

	sch.QcountState[ro]++

	if sch.Qqtable[Store{Row: ro, Col: 0}] == 0 && sch.Qqtable[Store{Row: ro, Col: 0}] == sch.Qqtable[Store{Row: ro, Col: 1}] {
		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	} else if sch.Qqtable[Store{Row: ro, Col: 0}] > sch.Qqtable[Store{Row: ro, Col: 1}] {
		action = 0
		action2 = 1
	} else {
		action = 1
		action2 = 0
	}

	var currentRTT time.Duration
	var lowerRTT time.Duration

	currentRTT = s.paths[availablePaths[action]].rttStats.SmoothedRTT()
	lowerRTT = s.paths[availablePaths[action2]].rttStats.SmoothedRTT()

	if (lowerRTT < currentRTT) && s.paths[availablePaths[action2]].SendingAllowed() {
		return s.paths[availablePaths[action2]]
	}

	if s.paths[availablePaths[action]].SendingAllowed() {
		//fmt.Println("qsat")
		return s.paths[availablePaths[action]]
	}

	if hasRetransmission && s.paths[protocol.InitialPathID].SendingAllowed() {
		return s.paths[protocol.InitialPathID]
	} else {
		//fmt.Println("nosat")
		return nil
	}

}

func (sch *scheduler) selectPathEarliestCompletionFirst(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var bestPath *path
	var secondBestPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	var secondLowerRTT time.Duration
	bestPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX: Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[bestPathID]
			if bestPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT >= lowerRTT {
			// Update second best available path
			if (secondLowerRTT == 0 || currentRTT < secondLowerRTT) && pth.SendingAllowed() {
				secondLowerRTT = currentRTT
				secondBestPath = pth
			}

			if currentRTT != 0 && lowerRTT != 0 && bestPath != nil {
				continue pathLoop
			}
		}

		// Update
		lowerRTT = currentRTT
		bestPath = pth
		bestPathID = pathID
	}

	// Unlikely
	if bestPath == nil {
		if secondBestPath != nil {
			return secondBestPath
		}
		return nil
	}

	// Allow retransmissions even if best path is blocked
	if hasRetransmission || bestPath.SendingAllowed() {
		return bestPath
	}

	// Stop looking if second best path is nil
	if secondBestPath == nil {
		return nil
	}

	// Else, check if it is beneficial to send on second best path
	var queueSize uint64
	getQueueSize := func(s *stream) (bool, error) {
		if s != nil {
			queueSize = queueSize + uint64(s.lenOfDataForWriting())
		}

		return true, nil
	}
	s.streamsMap.Iterate(getQueueSize)

	cwndBest := uint64(bestPath.GetCongestionWindow())
	cwndSecond := uint64(secondBestPath.GetCongestionWindow())
	deviationBest := uint64(bestPath.rttStats.MeanDeviation())
	deviationSecond := uint64(secondBestPath.rttStats.MeanDeviation())

	delta := deviationBest
	if deviationBest < deviationSecond {
		delta = deviationSecond
	}

	xBest := queueSize
	// if queueSize < cwndBest
	if queueSize > cwndBest {
		xBest = cwndBest
	}

	lhs := uint64(lowerRTT) * (xBest + cwndBest)
	rhs := cwndBest * (uint64(secondLowerRTT) + delta)
	if beta*lhs < (beta*rhs + uint64(sch.waiting)*rhs) {
		xSecond := queueSize
		if queueSize < cwndSecond {
			xSecond = cwndSecond
		}
		lhsSecond := uint64(secondLowerRTT) * xSecond
		rhsSecond := cwndSecond * (2*uint64(lowerRTT) + delta)

		if lhsSecond > rhsSecond {
			sch.waiting = 1
			return nil
		}
	} else {
		sch.waiting = 0
	}
	return secondBestPath
}

func (sch *scheduler) selectPathStreamAwareEarliestCompletionFirst(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	if s.streamScheduler == nil {
		return sch.selectPathEarliestCompletionFirst(s, hasRetransmission, hasStreamRetransmission, fromPth)
	}
	// Reset selected stream. Best path always sends next stream in turn
	s.streamScheduler.toSend = nil

	// Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var bestPath *path
	var secondBestPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	var secondLowerRTT time.Duration
	bestPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX: Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[bestPathID]
			if bestPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT >= lowerRTT {
			// Update second best available path
			if (secondLowerRTT == 0 || currentRTT < secondLowerRTT) && pth.SendingAllowed() {
				secondLowerRTT = currentRTT
				secondBestPath = pth
			}

			if currentRTT != 0 && lowerRTT != 0 && bestPath != nil {
				continue pathLoop
			}
		}

		// Update
		lowerRTT = currentRTT
		bestPath = pth
		bestPathID = pathID
	}

	// Unlikely
	if bestPath == nil {
		if secondBestPath != nil {
			return secondBestPath
		}
		return nil
	}

	// Allow retransmissions even if best path is blocked
	if hasRetransmission || bestPath.SendingAllowed() {
		return bestPath
	}

	// Stop looking if second best path is nil
	if secondBestPath == nil {
		return nil
	}

	var queueSize uint64
	getQueueSize := func(s *stream) (bool, error) {
		if s != nil {
			queueSize = queueSize + uint64(s.lenOfDataForWriting())
		}

		return true, nil
	}
	s.streamsMap.Iterate(getQueueSize)

	visited := make(map[protocol.StreamID]bool)
	i := 0
	for len(visited) < s.streamScheduler.openStreams && i < s.streamScheduler.openStreams /* Should find a better way to deal with blocked streams */ {
		i++

		strm := s.streamScheduler.schedule()
		if strm == nil {
			break
		}

		if visited[strm.id] {
			strm.skip()
			continue
		}
		visited[strm.id] = true

		k := uint64(s.streamScheduler.bytesUntilCompletion(strm))

		// To cope with streams that are about to finish
		if queueSize > k {
			queueSize = k
		}

		cwndBest := uint64(bestPath.GetCongestionWindow())
		cwndSecond := uint64(secondBestPath.GetCongestionWindow())
		deviationBest := uint64(bestPath.rttStats.MeanDeviation())
		deviationSecond := uint64(secondBestPath.rttStats.MeanDeviation())

		delta := deviationBest
		if deviationBest < deviationSecond {
			delta = deviationSecond
		}

		xBest := queueSize
		if queueSize > cwndBest {
			xBest = cwndBest
		}

		lhs := uint64(lowerRTT) * (xBest + cwndBest)
		rhs := cwndBest * (uint64(secondLowerRTT) + delta)
		if beta*lhs < (beta*rhs + uint64(strm.waiting)*rhs) {
			xSecond := queueSize
			if queueSize < cwndSecond {
				xSecond = cwndSecond
			}
			lhsSecond := uint64(secondLowerRTT) * xSecond
			rhsSecond := cwndSecond * (2*uint64(lowerRTT) + delta)

			if lhsSecond > rhsSecond {
				strm.waiting = 1
				continue
			}
		} else {
			strm.waiting = 0
		}
		return secondBestPath
	}

	return nil
}

func (sch *scheduler) selectPathFQSAT(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {

	//return s.paths[protocol.InitialPathID]

	// if sch.SchedulerName == "qsat" {
	// 	if rand.Float64() < sch.Epsilon {
	// 		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	// 	}
	// }else if sch.SchedulerName == "fuzzyqsat"{
	// 	ep_tmp := sch.Epsilon / (sch.AdaDivisor - 1/float64(sch.record))
	// 	if rand.Float64() < ep_tmp {
	// 		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	// 	}
	// }

	if rand.Float64() <= sch.Epsilon {
		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	}

	if len(s.paths) <= 1 {
		//fmt.Println("len1")
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	if len(s.paths) == 2 {
		//fmt.Println("len2")
		for pathID, path := range s.paths {
			if pathID != protocol.InitialPathID {
				utils.Debugf("Selecting path %d as unique path", pathID)
				return path
			}
		}
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)
	lRTT := make(map[protocol.PathID]time.Duration)
	cwnd := make(map[protocol.PathID]protocol.ByteCount)

	//Paths
	var availablePaths []protocol.PathID
	for pathID, pth := range s.paths {
		lRTT[pathID] = pth.rttStats.LatestRTT()
		cwnd[pathID] = pth.sentPacketHandler.GetCongestionWindow()
		if pathID != protocol.InitialPathID {
			// fmt.Println("PathID: ", pathID)
			availablePaths = append(availablePaths, pathID)
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

	f_sendingRate := (float64(cwnd[firstPath]) / float64(lRTT[firstPath])) / (float64(cwnd[firstPath])/float64(lRTT[firstPath]) + float64(cwnd[secondPath])/float64(lRTT[secondPath]))
	s_sendingRate := (float64(cwnd[secondPath]) / float64(lRTT[secondPath])) / (float64(cwnd[firstPath])/float64(lRTT[firstPath]) + float64(cwnd[secondPath])/float64(lRTT[secondPath]))

	//fmt.Println(firstPath, f_sendingRate, secondPath, s_sendingRate)

	var f_cLevel, s_cLevel int8
	var action, action2 protocol.PathID

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

	sch.countState[f_cLevel][s_cLevel]++

	if sch.qtable[f_cLevel][s_cLevel][0] == 0 && sch.qtable[f_cLevel][s_cLevel][1] == sch.qtable[f_cLevel][s_cLevel][0] {
		//fmt.Println("minrtt1")
		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	} else if sch.qtable[f_cLevel][s_cLevel][0] > sch.qtable[f_cLevel][s_cLevel][1] {
		action = 0
		action2 = 1
	} else {
		action = 1
		action2 = 0
	}

	if s.paths[availablePaths[action]].SendingAllowed() {
		//fmt.Println("qsat")
		return s.paths[availablePaths[action]]
	}

	var currentRTT time.Duration
	var lowerRTT time.Duration

	currentRTT = s.paths[availablePaths[action]].rttStats.SmoothedRTT()
	lowerRTT = s.paths[availablePaths[action2]].rttStats.SmoothedRTT()

	if (lowerRTT < currentRTT) && s.paths[availablePaths[action2]].SendingAllowed() {
		//fmt.Println("minrtt2")
		return s.paths[availablePaths[action2]]
	}

	return nil

	// if hasRetransmission && s.paths[protocol.InitialPathID].SendingAllowed(){
	// 	return s.paths[protocol.InitialPathID]
	// }else{
	// 	//fmt.Println("nosat")
	// 	return nil
	// }

}

// Lock of s.paths must be held
func (sch *scheduler) selectPath(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	return sch.pathScheduler(s, hasRetransmission, hasRetransmission, fromPth)
}

// Lock of s.paths must be free (in case of log print)
func (sch *scheduler) performPacketSending(s *session, windowUpdateFrames []*wire.WindowUpdateFrame, pth *path) (*ackhandler.Packet, bool, error) {
	// add a retransmittable frame
	if pth.sentPacketHandler.ShouldSendRetransmittablePacket() {
		s.packer.QueueControlFrame(&wire.PingFrame{}, pth)
	}
	packet, err := s.packer.PackPacket(pth, s)
	if err != nil || packet == nil {
		return nil, false, err
	}
	if err = s.sendPackedPacket(packet, pth); err != nil {
		return nil, false, err
	}

	// send every window update twice
	for _, f := range windowUpdateFrames {
		s.packer.QueueControlFrame(f, pth)
	}

	// Packet sent, so update its quota
	sch.quotas[pth.pathID]++

	// Provide some logging if it is the last packet
	for _, frame := range packet.frames {
		switch frame := frame.(type) {
		case *wire.StreamFrame:
			if frame.FinBit {
				// Last packet to send on the stream, print stats
				s.pathsLock.RLock()
				utils.Infof("Info for stream %x of %x", frame.StreamID, s.connectionID)
				for pathID, pth := range s.paths {
					sntPkts, sntRetrans, sntLost := pth.sentPacketHandler.GetStatistics()
					rcvPkts := pth.receivedPacketHandler.GetStatistics()
					utils.Infof("Path %x: sent %d retrans %d lost %d; rcv %d rtt %v", pathID, sntPkts, sntRetrans, sntLost, rcvPkts, pth.rttStats.SmoothedRTT())
				}
				s.pathsLock.RUnlock()
			}
			// else {
			// 	fmt.Println("StreamInfo2: ", frame.StreamID, pth.pathID)
			// 	// if s.perspective == protocol.PerspectiveServer {
			// 	// 	if _, err := os.Stat("./logs/server-detail.logs"); err == nil {
			// 	// 		file, _ := os.OpenFile("./logs/server-detail.logs", os.O_APPEND|os.O_WRONLY, 0644)
			// 	// 		defer file.Close()
			// 	// 		fmt.Fprintf(file, "Sending for stream %d of weight %d\n", frame.StreamID, frame.StreamID)
			// 	// 	}
			// 	// }
			// }
		default:
		}
	}

	pkt := &ackhandler.Packet{
		PacketNumber:    packet.number,
		Frames:          packet.frames,
		Length:          protocol.ByteCount(len(packet.raw)),
		EncryptionLevel: packet.encryptionLevel,
	}

	return pkt, true, nil
}

// Lock of s.paths must be free
func (sch *scheduler) ackRemainingPaths(s *session, totalWindowUpdateFrames []*wire.WindowUpdateFrame) error {
	// Either we run out of data, or CWIN of usable paths are full
	// Send ACKs on paths not yet used, if needed. Either we have no data to send and
	// it will be a pure ACK, or we will have data in it, but the CWIN should then
	// not be an issue.
	s.pathsLock.RLock()
	defer s.pathsLock.RUnlock()
	// get WindowUpdate frames
	// this call triggers the flow controller to increase the flow control windows, if necessary
	windowUpdateFrames := totalWindowUpdateFrames
	if len(windowUpdateFrames) == 0 {
		windowUpdateFrames = s.getWindowUpdateFrames(s.peerBlocked)
	}
	for _, pthTmp := range s.paths {
		ackTmp := pthTmp.GetAckFrame()
		for _, wuf := range windowUpdateFrames {
			s.packer.QueueControlFrame(wuf, pthTmp)
		}
		if ackTmp != nil || len(windowUpdateFrames) > 0 {
			if pthTmp.pathID == protocol.InitialPathID && ackTmp == nil {
				continue
			}
			swf := pthTmp.GetStopWaitingFrame(false)
			if swf != nil {
				s.packer.QueueControlFrame(swf, pthTmp)
			}
			s.packer.QueueControlFrame(ackTmp, pthTmp)
			// XXX (QDC) should we instead call PackPacket to provides WUFs?
			var packet *packedPacket
			var err error
			if ackTmp != nil {
				// Avoid internal error bug
				packet, err = s.packer.PackAckPacket(pthTmp)
			} else {
				packet, err = s.packer.PackPacket(pthTmp, s)
			}
			if err != nil {
				return err
			}
			err = s.sendPackedPacket(packet, pthTmp)
			if err != nil {
				return err
			}
		}
	}
	s.peerBlocked = false
	return nil
}

func (sch *scheduler) sendPacket(s *session) error {
	var pth *path

	// Update leastUnacked value of paths
	s.pathsLock.RLock()
	for _, pthTmp := range s.paths {
		pthTmp.SetLeastUnacked(pthTmp.sentPacketHandler.GetLeastUnacked())
	}
	s.pathsLock.RUnlock()

	// get WindowUpdate frames
	// this call triggers the flow controller to increase the flow control windows, if necessary
	windowUpdateFrames := s.getWindowUpdateFrames(false)
	for _, wuf := range windowUpdateFrames {
		s.packer.QueueControlFrame(wuf, pth)
	}

	// Repeatedly try sending until we don't have any more data, or run out of the congestion window
	for {
		// We first check for retransmissions
		hasRetransmission, retransmitHandshakePacket, fromPth := sch.getRetransmission(s)
		// XXX There might still be some stream frames to be retransmitted
		hasStreamRetransmission := s.streamFramer.HasFramesForRetransmission()

		// Select the path here
		s.pathsLock.RLock()
		pth = sch.selectPath(s, hasRetransmission, hasStreamRetransmission, fromPth)
		s.pathsLock.RUnlock()

		// XXX No more path available, should we have a new QUIC error message?
		if pth == nil {
			windowUpdateFrames := s.getWindowUpdateFrames(false)
			return sch.ackRemainingPaths(s, windowUpdateFrames)
		}

		// If we have an handshake packet retransmission, do it directly
		if hasRetransmission && retransmitHandshakePacket != nil {
			s.packer.QueueControlFrame(pth.sentPacketHandler.GetStopWaitingFrame(true), pth)
			packet, err := s.packer.PackHandshakeRetransmission(retransmitHandshakePacket, pth)
			if err != nil {
				return err
			}
			if err = s.sendPackedPacket(packet, pth); err != nil {
				return err
			}
			continue
		}

		// XXX Some automatic ACK generation should be done someway
		var ack *wire.AckFrame

		ack = pth.GetAckFrame()
		if ack != nil {
			s.packer.QueueControlFrame(ack, pth)
		}
		if ack != nil || hasStreamRetransmission {
			swf := pth.sentPacketHandler.GetStopWaitingFrame(hasStreamRetransmission)
			if swf != nil {
				s.packer.QueueControlFrame(swf, pth)
			}
		}

		// Also add CLOSE_PATH frames, if any
		for cpf := s.streamFramer.PopClosePathFrame(); cpf != nil; cpf = s.streamFramer.PopClosePathFrame() {
			s.packer.QueueControlFrame(cpf, pth)
		}

		// Also add ADD ADDRESS frames, if any
		for aaf := s.streamFramer.PopAddAddressFrame(); aaf != nil; aaf = s.streamFramer.PopAddAddressFrame() {
			s.packer.QueueControlFrame(aaf, pth)
		}

		// Also add PATHS frames, if any
		for pf := s.streamFramer.PopPathsFrame(); pf != nil; pf = s.streamFramer.PopPathsFrame() {
			s.packer.QueueControlFrame(pf, pth)
		}

		pkt, sent, err := sch.performPacketSending(s, windowUpdateFrames, pth)
		if err != nil {
			return err
		}
		windowUpdateFrames = nil
		if !sent {
			// Prevent sending empty packets
			return sch.ackRemainingPaths(s, windowUpdateFrames)
		}

		if sch.SchedulerName == "SAC" && pth.pathID > 0 && pkt.PacketNumber > 0 {
			sch.list_State_DQN[State{pth.pathID, pkt.PacketNumber}] = sch.current_State_DQN
			sch.list_Action_DQN[State{pth.pathID, pkt.PacketNumber}] = sch.current_Prob
			//fmt.Println(sch.current_Prob)
		}

		if sch.SchedulerName == "QSAT" && pth.pathID > 0 && pkt.PacketNumber > 0 {
			BSend, _ := s.flowControlManager.SendWindowSize(protocol.StreamID(5))
			BSend1 := float32(BSend) / float32(protocol.DefaultMaxCongestionWindow) / 300
			sch.QoldState[State{id: pth.pathID, pktnumber: pkt.PacketNumber}] = float64(BSend1)
		}

		// Duplicate traffic when it was sent on an unknown performing path
		// FIXME adapt for new paths coming during the connection
		if pth.rttStats.SmoothedRTT() == 0 {
			currentQuota := sch.quotas[pth.pathID]
			// Was the packet duplicated on all potential paths?
		duplicateLoop:
			for pathID, tmpPth := range s.paths {
				if pathID == protocol.InitialPathID || pathID == pth.pathID {
					continue
				}
				if sch.quotas[pathID] < currentQuota && tmpPth.sentPacketHandler.SendingAllowed() {
					// Duplicate it
					pth.sentPacketHandler.DuplicatePacket(pkt)
					break duplicateLoop
				}
			}
		}

		// And try pinging on potentially failed paths
		if fromPth != nil && fromPth.potentiallyFailed.Get() {
			err = s.sendPing(fromPth)
			if err != nil {
				return err
			}
		}
	}
}

// func (sch *scheduler) selectPathSAC(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
// 	if s.perspective == protocol.PerspectiveClient {
// 		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
// 	}
// 	// if rand.Float64() <= sch.Epsilon {
// 	// 	return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
// 	// }
// 	//fmt.Println("CHECKKK")

// 	if len(s.paths) <= 1 {
// 		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
// 			return nil
// 		}
// 		return s.paths[protocol.InitialPathID]
// 	}

// 	if len(s.paths) == 2 {
// 		for pathID, path := range s.paths {
// 			if pathID != protocol.InitialPathID {
// 				utils.Debugf("Selecting path %d as unique path", pathID)
// 				return path
// 			}
// 		}
// 	}

// 	// FIXME Only works at the beginning... Cope with new paths during the connection
// 	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
// 		// Is there any other path with a lower number of packet sent?
// 		currentQuota := sch.quotas[fromPth.pathID]
// 		for pathID, pth := range s.paths {
// 			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
// 				continue
// 			}
// 			// The congestion window was checked when duplicating the packet
// 			if sch.quotas[pathID] < currentQuota {
// 				return pth
// 			}
// 		}
// 	}

// 	if sch.current_Prob == 0 {
// 		sch.getActionAsync(baseURL+"/get_action", s.paths)
// 		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
// 	} else {
// 		if sch.current_Action != 1 && sch.current_Action != 3 {
// 			fmt.Println("Loiii: ", sch.current_Action)
// 		}
// 		if s.paths[sch.current_Action].SendingAllowed() {
// 			return s.paths[sch.current_Action]
// 		}
// 		// else {
// 		// 	sch.getActionAsync(baseURL+"/get_action", s.paths)
// 		// }
// 	}

// 	// if s.paths[sch.current_Action].SendingAllowed() {
// 	// 	// fmt.Println("Selected path: ", s.paths[sch.current_Action], sch.current_Prob)
// 	// 	return s.paths[sch.current_Action]
// 	// }

// 	// if s.paths[sch.current_Action].SendingAllowed() {
// 	// 	fmt.Println("Selected path: ", s.paths[sch.current_Action], sch.current_Prob)
// 	// 	return s.paths[sch.current_Action]
// 	// }

// 	// src := rand.NewSource(time.Now().UnixNano())
// 	// r := rand.New(src)

// 	// action := 1
// 	// if sch.current_Prob > r.Float64() {
// 	// 	action = 3
// 	// }
// 	// sch.current_Action = protocol.PathID(action)
// 	// if s.paths[sch.current_Action].SendingAllowed() {
// 	// 	return s.paths[sch.current_Action]
// 	// }

// 	// fmt.Println("Un-path ")
// 	// sch.getActionAsync(baseURL+"/get_action", stateData)
// 	// if sch.current_Prob > r.Float64() {
// 	// 	action = 1
// 	// }
// 	// //fmt.Println("Action: ", action, "Prob: ", sch.current_Prob)

// 	// if s.paths[availablePaths[action]].SendingAllowed() {
// 	// 	sch.current_State_DQN = stateData
// 	// 	return s.paths[availablePaths[action]]
// 	// }
// 	var currentRTT time.Duration
// 	var lowerRTT time.Duration
// 	var secondPath protocol.PathID
// 	if sch.current_Action == 1 {
// 		secondPath = 3
// 	} else {
// 		secondPath = 1
// 	}
// 	currentRTT = s.paths[sch.current_Action].rttStats.SmoothedRTT()
// 	lowerRTT = s.paths[secondPath].rttStats.SmoothedRTT()

// 	if (lowerRTT < currentRTT) && s.paths[secondPath].SendingAllowed() {
// 		//fmt.Println("minrtt2")
// 		return s.paths[secondPath]
// 	}

// 	if hasRetransmission && s.paths[protocol.InitialPathID].SendingAllowed() {
// 		return s.paths[protocol.InitialPathID]
// 	} else {
// 		return nil
// 	}

// }

// func (sch *scheduler) getActionAsync(url string, paths map[protocol.PathID]*path) {
// 	go func() {
// 		firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)
// 		sRTT := make(map[protocol.PathID]time.Duration)
// 		lRTT := make(map[protocol.PathID]time.Duration)
// 		CWND := make(map[protocol.PathID]protocol.ByteCount)
// 		INP := make(map[protocol.PathID]protocol.ByteCount)

// 		//Paths
// 		var availablePaths []protocol.PathID
// 		for pathID, pth := range paths {
// 			CWND[pathID] = pth.sentPacketHandler.GetCongestionWindow()
// 			INP[pathID] = pth.sentPacketHandler.GetBytesInFlight()
// 			sRTT[pathID] = pth.rttStats.SmoothedRTT()
// 			lRTT[pathID] = pth.rttStats.LatestRTT()
// 			if pathID != protocol.InitialPathID {
// 				availablePaths = append(availablePaths, pathID)
// 				if firstPath == protocol.PathID(255) {
// 					firstPath = pathID
// 				} else {
// 					if pathID < firstPath {
// 						secondPath = firstPath
// 						firstPath = pathID
// 					} else {
// 						secondPath = pathID
// 					}
// 				}
// 			}

// 		}

// 		stateData := StateDQN{
// 			CWNDf: float64(CWND[firstPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
// 			INPf:  float64(INP[firstPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
// 			SRTTf: NormalizeTimes(sRTT[firstPath]) / 50.0,
// 			CWNDs: float64(CWND[secondPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
// 			INPs:  float64(INP[secondPath]) / float64(protocol.DefaultMaxCongestionWindow*1024),
// 			SRTTs: NormalizeTimes(sRTT[secondPath]) / 50.0,
// 		}

// 		jsonPayload, err := json.Marshal(map[string]interface{}{
// 			"state": stateData,
// 		})
// 		if err != nil {
// 			fmt.Println("Error encoding JSON:", err)
// 			return
// 		}

// 		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
// 		if err != nil {
// 			fmt.Println("Error sending POST request:", err)
// 			return
// 		}
// 		defer resp.Body.Close()

// 		var response ActionProbabilityResponse
// 		err = json.NewDecoder(resp.Body).Decode(&response)
// 		if err != nil {
// 			fmt.Println("Error decoding response:", err)
// 			return
// 		}

// 		if response.Error != "" {
// 			fmt.Println("Server returned error:", response.Error)
// 			return
// 		}

// 		// fmt.Println("Received action probability:", response.Probability[0])
// 		sch.current_Prob = response.Probability[0]
// 		src := rand.NewSource(time.Now().UnixNano())
// 		r := rand.New(src)

// 		action := 1
// 		if sch.current_Prob > r.Float64() {
// 			action = 3
// 		}
// 		sch.current_Action = protocol.PathID(action)
// 		sch.current_State_DQN = stateData
// 		// sch.flag_train = false
// 	}()
// }

func (sch *scheduler) selectPathSAC(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// if rand.Float64() <= sch.Epsilon {
	// 	return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	// }
	//fmt.Println("CHECKKK")

	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	if len(s.paths) == 2 {
		for pathID, path := range s.paths {
			if pathID != protocol.InitialPathID {
				utils.Debugf("Selecting path %d as unique path", pathID)
				return path
			}
		}
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)
	sRTT := make(map[protocol.PathID]time.Duration)
	lRTT := make(map[protocol.PathID]time.Duration)
	CWND := make(map[protocol.PathID]protocol.ByteCount)
	INP := make(map[protocol.PathID]protocol.ByteCount)

	//Paths
	var availablePaths []protocol.PathID
	for pathID, pth := range s.paths {
		CWND[pathID] = pth.sentPacketHandler.GetCongestionWindow()
		INP[pathID] = pth.sentPacketHandler.GetBytesInFlight()
		sRTT[pathID] = pth.rttStats.SmoothedRTT()
		lRTT[pathID] = pth.rttStats.LatestRTT()
		if pathID != protocol.InitialPathID {
			availablePaths = append(availablePaths, pathID)
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

	stateData := StateDQN{
		CWNDf: float64(CWND[firstPath]) / float64(protocol.DefaultTCPMSS),
		INPf:  float64(INP[firstPath]) / float64(protocol.DefaultTCPMSS),
		SRTTf: NormalizeTimes(sRTT[firstPath]) / 10,
		CWNDs: float64(CWND[secondPath]) / float64(protocol.DefaultTCPMSS),
		INPs:  float64(INP[secondPath]) / float64(protocol.DefaultTCPMSS),
		SRTTs: NormalizeTimes(sRTT[secondPath]) / 10,
	}

	if sch.current_Prob == 0 {
		sch.current_Prob = 1
		// rewardPayload := sch.list_Reward_DQN[sch.current_Prob]
		// rewardPayload.NextState = stateData
		// sch.list_Reward_DQN[sch.current_Prob] = rewardPayload
		// fmt.Println("State: ", stateData)

		sch.getActionAsync(baseURL+"/get_action", stateData, sch.model_id)
		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	} else if sch.current_Prob == 1 {
		return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	}

	// elapsed := time.Since(sch.time_Get_Action)

	if sch.count_Action >= uint16(sch.Alpha) {
		// fmt.Println("PayLoad: ", rewardPayload)
		// rewardPayload := sch.list_Reward_DQN[sch.current_Prob]
		// rewardPayload.NextState = stateData
		// sch.list_Reward_DQN[sch.current_Prob] = rewardPayload
		var connID string = strconv.FormatFloat(sch.current_Prob, 'E', -1, 64)
		if tmp_Reload, ok := multiclients.List_Reward_DQN.Get(connID); ok {
			rewardPayload, ok := tmp_Reload.(RewardPayload)
			if !ok {
				fmt.Println("Type assertion failed")
				return nil
			}
			rewardPayload.NextState = stateData
			multiclients.List_Reward_DQN.Set(connID, rewardPayload)
			// fmt.Println("State: ", stateData, rewardPayload)
		}

		sch.current_State_DQN = stateData
		sch.getActionAsync(baseURL+"/get_action", stateData, sch.model_id)

		sch.count_Action = 0
		sch.count_Reward = 0
		sch.current_Reward = 0
		sch.time_Get_Action = time.Now()
	} else {
		sch.count_Action += 1
	}

	action := 0
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)

	if sch.current_Prob > r.Float64() {
		action = 1
	}
	if s.paths[availablePaths[action]].SendingAllowed() {
		// sch.current_State_DQN = stateData
		return s.paths[availablePaths[action]]
	}

	if hasRetransmission && s.paths[protocol.InitialPathID].SendingAllowed() {
		return s.paths[protocol.InitialPathID]
	} else {
		return nil
	}

}

func (sch *scheduler) getActionAsync(url string, state StateDQN, model_id uint64) {
	go func() {
		// time_now := time.Now()
		jsonPayload, err := json.Marshal(map[string]interface{}{
			"state":    state,
			"model_id": model_id,
		})

		// fmt.Println("GetAction: ", model_id)
		if err != nil {
			fmt.Println("Error encoding JSON:", err)
			return
		}

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			fmt.Println("Error sending POST request:", err)
			return
		}
		defer resp.Body.Close()

		var response ActionProbabilityResponse
		err = json.NewDecoder(resp.Body).Decode(&response)
		if err != nil {
			fmt.Println("Error decoding response:", err)
			return
		}

		if response.Error != "" {
			fmt.Println("Server returned error:", response.Error)
			return
		}

		// fmt.Println("Received action probability:", response.Probability[0])
		sch.current_Prob = response.Probability[0]

		rewardPayload := RewardPayload{
			State:       state,
			NextState:   state,
			Action:      sch.current_Prob,
			Reward:      0.0,
			Done:        false,
			ModelID:     sch.model_id,
			CountReward: 0,
		}
		var connID string = strconv.FormatFloat(sch.current_Prob, 'E', -1, 64)
		multiclients.List_Reward_DQN.Set(connID, rewardPayload)
		// elaped := time.Since(time_now).Milliseconds()
		// fmt.Println("Time_getAction: ", elaped)
	}()
}
