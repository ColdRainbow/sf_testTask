package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/go-chi/chi/v5"
)

const (
	SLOTS_PER_EPOCH  = 32
	SECONDS_PER_SLOT = 12
	RPC_ENDPOINT     = "https://methodical-billowing-dew.quiknode.pro/d23a8baebb4c5f2c1e0c25e20655e66a48a5873e"
)

type BlockRewardResponse struct {
	Status string `json:"status"`
	Reward string `json:"reward"`
}

type SyncDutiesResponse struct {
	Validators []string `json:"validators"`
}

type beaconBlock struct {
	Data struct {
		Message struct {
			Slot string `json:"slot"`
			Body struct {
				Execution_payload struct {
					Block_number string `json:"block_number"`
				} `json:"execution_payload"`
			} `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

type syncCommittee struct {
	Data struct {
		Validators []string `json:"validators"`
	} `json:"data"`
}

type syncDuties struct {
	Data []struct {
		Pubkey string `json:"pubkey"`
	} `json:"data"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

func getBeaconBlock(w http.ResponseWriter, r *http.Request, slot string) (beaconBlock, error) {
	blockReward := beaconBlock{}
	reqGetBeaconBlock, err := http.NewRequestWithContext(r.Context(), "GET", RPC_ENDPOINT+"/eth/v2/beacon/blocks/"+slot, nil)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getBeaconBlock: failed to create request")
		return blockReward, err
	}

	respGetBeaconBlock, err := http.DefaultClient.Do(reqGetBeaconBlock)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getBeaconBlock: failed to get beacon block")
		return blockReward, err
	}
	defer respGetBeaconBlock.Body.Close()
	if respGetBeaconBlock.StatusCode != http.StatusOK {
		respondWithError(w, http.StatusNotFound, "getBeaconBlock: failed to find beacon block")
		return blockReward, err
	}

	err = json.NewDecoder(respGetBeaconBlock.Body).Decode(&blockReward)
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "getBeaconBlock: failed to decode beacon block")
		return blockReward, err
	}

	return blockReward, nil
}

func getCurrentSlot(w http.ResponseWriter, r *http.Request) (uint64, error) {
	blockReward, err := getBeaconBlock(w, r, "head")
	if err != nil {
		return 0, err
	}

	slot, err := strconv.ParseUint(blockReward.Data.Message.Slot, 10, 64)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getCurrentSlot: failed to parse slot")
	}
	return slot, nil
}

func getBlockReward(w http.ResponseWriter, r *http.Request) {
	slot := chi.URLParam(r, "slot")
	slotInt, err := strconv.ParseUint(slot, 10, 64)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getBlockReward: failed to parse slot")
		return
	}

	currentSlot, err := getCurrentSlot(w, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getBlockReward: failed to get current slot")
		return
	}
	if slotInt > currentSlot {
		respondWithError(w, http.StatusBadRequest, "getBlockReward: invalid slot")
		return
	}

	blockReward, err := getBeaconBlock(w, r, slot)
	if err != nil {
		return
	}

	blockNumber := blockReward.Data.Message.Body.Execution_payload.Block_number
	client, err := ethclient.Dial(RPC_ENDPOINT)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getBlockReward: failed to connect to RPC")
		return
	}

	blockNumberInt, err := strconv.ParseInt(blockNumber, 10, 64)

	block, err := client.BlockByNumber(context.Background(), big.NewInt(blockNumberInt))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getBlockReward: failed to get block")
		return
	}
	extra := block.Extra()
	isMEV := bytes.Contains(extra, []byte("build"))

	reward := big.NewFloat(0.0)
	transactions := block.Transactions()
	if isMEV {
		lastTransaction := transactions[len(transactions)-1]
		reward.SetInt(lastTransaction.Value())
	} else {
		recepits, err := client.BlockReceipts(r.Context(), rpc.BlockNumberOrHashWithHash(block.Hash(), false))
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "getBlockReward: failed to get receipts")
		}

		fees := new(big.Int)
		for _, receipt := range recepits {
			fees.Add(fees, new(big.Int).Mul(receipt.EffectiveGasPrice, new(big.Int).SetUint64(receipt.GasUsed)))
		}
		burned := new(big.Int).SetUint64(block.GasUsed())
		burned.Mul(burned, block.BaseFee())
		fees.Sub(fees, burned)
		reward = reward.Quo(big.NewFloat(0.0).SetInt(fees), big.NewFloat(0.0).SetInt64(params.GWei))
	}

	response := BlockRewardResponse{
		Status: map[bool]string{true: "mev", false: "vanilla"}[isMEV],
		Reward: reward.Text('f', -1),
	}

	json.NewEncoder(w).Encode(response)
}

func getSyncDuties(w http.ResponseWriter, r *http.Request) {
	syncCommittee := syncCommittee{}
	syncDuties := syncDuties{}
	pubkeys := []string{}
	slot := chi.URLParam(r, "slot")
	slotInt, err := strconv.ParseUint(slot, 10, 64)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getSyncDuties: invalid slot in getSyncDuties")
		return
	}

	currentSlot, err := getCurrentSlot(w, r)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getSyncDuties: failed to get current slot")
		return
	}
	if slotInt > currentSlot {
		respondWithError(w, http.StatusBadRequest, "getSyncDuties: invalid slot")
		return
	}

	req, _ := http.NewRequest("GET", RPC_ENDPOINT+"/eth/v1/beacon/states/"+slot+"/sync_committees", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getSyncDuties: failed to create request")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respondWithError(w, http.StatusNotFound, "getSyncDuties: failed to find sync_committees")
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&syncCommittee)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getSyncDuties: failed to decode syncCommittee")
		return
	}

	validatorIndices, err := json.Marshal(syncCommittee.Data.Validators)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getSyncDuties: failed to marshal validator indices")
		return
	}

	epochInt := slotInt / SLOTS_PER_EPOCH
	epoch := strconv.FormatUint(epochInt, 10)

	req, _ = http.NewRequest("POST", RPC_ENDPOINT+"/eth/v1/validator/duties/sync/"+epoch, bytes.NewReader(validatorIndices))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getSyncDuties: failed to execute request")
		return
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&syncDuties)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "getSyncDuties: failed to decode syncDuties")
		return
	}

	for i := 0; i < len(syncDuties.Data); i++ {
		pubkey := syncDuties.Data[i].Pubkey
		pubkeys = append(pubkeys, pubkey)
	}

	response := SyncDutiesResponse{
		Validators: pubkeys,
	}

	json.NewEncoder(w).Encode(response)
}

func main() {
	r := chi.NewRouter()
	r.Get("/blockreward/{slot}", getBlockReward)
	r.Get("/syncduties/{slot}", getSyncDuties)

	http.ListenAndServe(":8080", r)
}
