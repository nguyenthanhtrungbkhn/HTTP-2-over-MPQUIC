package quic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/lucas-clemente/quic-go/internal/protocol"
)

// const banditAlpha = 0.75
const banditDimension = 6
const baseURL = "http://127.0.0.1:8080"

var (
	SV_Txbitrate_interface0 float64
	SV_Txbitrate_interface1 float64
)

type StateDQN struct {
	CWNDf float64 `json:"CWNDf"`
	INPf  float64 `json:"INPf"`
	SRTTf float64 `json:"SRTTf"`
	CWNDs float64 `json:"CWNDs"`
	INPs  float64 `json:"INPs"`
	SRTTs float64 `json:"SRTTs"`
}

type UpdateDataDQN struct {
	State     StateDQN `json:"state"`
	Action    int      `json:"action"`
	Reward    float64  `json:"reward"`
	NextState StateDQN `json:"next_state"`
	Done      bool     `json:"done"`
}

// ActionProbabilityResponse represents the response structure for /get_action endpoint
type ActionProbabilityResponse struct {
	Probability []float64 `json:"probability"`
	Error       string    `json:"error"`
}

// RewardPayload represents the payload structure for /update_reward endpoint
type RewardPayload struct {
	State       StateDQN `json:"state"`
	NextState   StateDQN `json:"next_state"`
	Action      float64  `json:"action"`
	Reward      float64  `json:"reward"`
	Done        bool     `json:"done"`
	ModelID     uint64   `json:"model_id"`
	CountReward uint16   `json:"count_reward"`
}

// StatusResponse represents the response structure for /status endpoint
type StatusResponse struct {
	Status string `json:"status"`
}

type State struct {
	id        protocol.PathID
	pktnumber protocol.PacketNumber
}

type StateSACMulti struct {
	CWNDf     float64 `json:"CWNDf"`
	INPf      float64 `json:"INPf"`
	SRTTf     float64 `json:"SRTTf"`
	CWNDs     float64 `json:"CWNDs"`
	INPs      float64 `json:"INPs"`
	SRTTs     float64 `json:"SRTTs"`
	CWNDf_all float64 `json:"CWNDf_all"`
	INPf_all  float64 `json:"INPf_all"`
	SRTTf_all float64 `json:"SRTTf_all"`
	CWNDs_all float64 `json:"CWNDs_all"`
	INPs_all  float64 `json:"INPs_all"`
	SRTTs_all float64 `json:"SRTTs_all"`
	CNumber   int     `json:"CNumber"`
}

type UpdateDataSACMulti struct {
	State     StateSACMulti `json:"state"`
	Action    int           `json:"action"`
	Reward    float64       `json:"reward"`
	NextState StateSACMulti `json:"next_state"`
	Done      bool          `json:"done"`
}

// RewardPayload represents the payload structure for /update_reward endpoint
type RewardPayloadSACMulti struct {
	State       StateSACMulti `json:"state"`
	NextState   StateSACMulti `json:"next_state"`
	Action      float64       `json:"action"`
	Reward      float64       `json:"reward"`
	Reward_tmp  float64       `json:"reward_tmp"`
	Done        bool          `json:"done"`
	ModelID     uint64        `json:"model_id"`
	CountReward uint16        `json:"count_reward"`
}

type ActionJoinCC struct {
	Action1 float64 `json:"action_1"`
	Action2 float64 `json:"action_2"`
	Action3 float64 `json:"action_3"`
}

// RewardPayload represents the payload structure for /update_reward endpoint
type RewardPayloadSACMultiJoinCC struct {
	State       StateSACMulti `json:"state"`
	NextState   StateSACMulti `json:"next_state"`
	Action      ActionJoinCC  `json:"action"`
	Reward      float64       `json:"reward"`
	Done        bool          `json:"done"`
	ModelID     uint64        `json:"model_id"`
	CountReward uint16        `json:"count_reward"`
}

func setModel(modelType string, modelID uint64) {
	url := "http://localhost:8080/set_model"
	data := map[string]string{
		"model_type": modelType,
		"model_id":   strconv.FormatUint(modelID, 10), // Convert uint64 to string
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}
	fmt.Println("CHECKKKK: ", data)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error making POST request:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("Model set to SAC successfully", resp.Body)
	} else {
		fmt.Println("Failed to set model:", resp.Status)
	}
}
