package shard_stake

import (
	"encoding/hex"
	"fmt"
	"github.com/ontio/ontology-crypto/keypair"
	"github.com/ontio/ontology/common"
	"github.com/ontio/ontology/core/types"
	"github.com/ontio/ontology/smartcontract/service/native"
)

func commitDpos(native *native.NativeService, shardId types.ShardID, feeInfo map[string]uint64, view View) error {
	currentView, err := getShardCurrentView(native, shardId)
	if err != nil {
		return fmt.Errorf("commitDpos: get shard %d current view failed, err: %s", shardId, err)
	}
	if view != currentView {
		return fmt.Errorf("commitDpos: the view %d not equals current view %d", view, currentView)
	}
	// TODO: check should current+1 or current+2
	nextView := currentView + 1
	currentViewInfo, err := GetShardViewInfo(native, shardId, currentView)
	if err != nil {
		return fmt.Errorf("commitDpos: get shard %d current view info failed, err: %s", shardId, err)
	}
	nextViewInfo, err := GetShardViewInfo(native, shardId, nextView)
	if err != nil {
		return fmt.Errorf("commitDpos: get shard %d next view info failed, err: %s", shardId, err)
	}
	if nextViewInfo.Peers == nil || len(nextViewInfo.Peers) == 0 {
		nextViewInfo = currentViewInfo
		err = setShardViewInfo(native, shardId, nextView, nextViewInfo)
		if err != nil {
			return fmt.Errorf("commitDpos: update shard %d next view info failed, err: %s", shardId, err)
		}
	}
	for pubKeyString, feeAmount := range feeInfo {
		pubKeyData, err := hex.DecodeString(pubKeyString)
		if err != nil {
			return fmt.Errorf("commitDpos: decode pub key %s failed, err: %s", pubKeyString, err)
		}
		peer, err := keypair.DeserializePublicKey(pubKeyData)
		if err != nil {
			return fmt.Errorf("commitDpos: deserialize pub key %s failed, err: %s", pubKeyString, err)
		}
		peerInfo, ok := currentViewInfo.Peers[peer]
		if !ok {
			return fmt.Errorf("commitDpos: peer %s not exist at current view", pubKeyString)
		}
		peerInfo.WholeFee = feeAmount
		peerInfo.FeeBalance = feeAmount
		currentViewInfo.Peers[peer] = peerInfo
	}
	err = setShardViewInfo(native, shardId, currentView, currentViewInfo)
	if err != nil {
		return fmt.Errorf("commitDpos: update shard %d view info failed, err: %s", shardId, err)
	}
	err = setShardView(native, shardId, nextView)
	if err != nil {
		return fmt.Errorf("commitDpos: update shard %d view failed, err: %s", shardId, err)
	}
	return nil
}

func peerStake(native *native.NativeService, id types.ShardID, peerPubKey keypair.PublicKey, peerOwner common.Address,
	amount uint64) error {
	initView := View(0)
	info := &UserStakeInfo{Peers: make(map[keypair.PublicKey]*UserPeerStakeInfo)}
	info.Peers[peerPubKey] = &UserPeerStakeInfo{StakeAmount: amount}
	err := setShardViewUserStake(native, id, initView, peerOwner, info)
	if err != nil {
		return fmt.Errorf("peerStake: set init view peer stake info failed, err: %s", err)
	}
	nextView := initView + 1
	err = setShardViewUserStake(native, id, nextView, peerOwner, info)
	if err != nil {
		return fmt.Errorf("peerStake: set next view peer stake info failed, err: %s", err)
	}
	initViewInfo, err := GetShardViewInfo(native, id, initView)
	if err != nil {
		return fmt.Errorf("peerStake: get init view info failed, err: %s", err)
	}
	nextViewInfo, err := GetShardViewInfo(native, id, nextView)
	if err != nil {
		return fmt.Errorf("peerStake: get next view info failed, err: %s", err)
	}
	initViewInfo.Peers[peerPubKey].WholeStakeAmount = initViewInfo.Peers[peerPubKey].WholeStakeAmount + amount
	err = setShardViewInfo(native, id, initView, initViewInfo)
	if err != nil {
		return fmt.Errorf("peerStake: update init view info failed, err: %s", err)
	}
	nextViewInfo.Peers[peerPubKey].WholeStakeAmount = initViewInfo.Peers[peerPubKey].WholeStakeAmount + amount
	err = setShardViewInfo(native, id, nextView, nextViewInfo)
	if err != nil {
		return fmt.Errorf("peerStake: update current view info failed, err: %s", err)
	}
	// update user last stake view num
	err = setUserLastStakeView(native, id, peerOwner, nextView)
	if err != nil {
		return fmt.Errorf("peerStake: failed, err: %s", err)
	}
	return nil
}

func userStake(native *native.NativeService, id types.ShardID, user common.Address, stakeInfo map[string]uint64) error {
	// get view index
	lastStakeView, err := getUserLastStakeView(native, id, user)
	if err != nil {
		return fmt.Errorf("userStake: failed, err: %s", err)
	}
	currentView, err := getShardCurrentView(native, id)
	if err != nil {
		return fmt.Errorf("userStake: failed, err: %s", err)
	}
	nextView := currentView + 1

	userStakeInfo, err := getShardViewUserStake(native, id, lastStakeView, user)
	if err != nil {
		return fmt.Errorf("userStake: failed, err: %s", err)
	}
	shardViewInfo, err := GetShardViewInfo(native, id, nextView)
	if err != nil {
		return fmt.Errorf("userStake: failed, err: %s", err)
	}

	// update user current stake info
	if lastStakeView < currentView {
		err = setShardViewUserStake(native, id, currentView, user, userStakeInfo)
		if err != nil {
			return fmt.Errorf("userStake: set current view user stake info failed, err: %s", err)
		}
	} else if lastStakeView > nextView {
		return fmt.Errorf("userStake: user last stake view %d and next view %d unmatch", lastStakeView, nextView)
	}

	for pubKeyString, amount := range stakeInfo {
		pubKeyData, err := hex.DecodeString(pubKeyString)
		if err != nil {
			return fmt.Errorf("userStake: decode pub key %s failed, err: %s", pubKeyString, err)
		}
		peer, err := keypair.DeserializePublicKey(pubKeyData)
		if err != nil {
			return fmt.Errorf("userStake: deserialize pub key %s failed, err: %s", pubKeyString, err)
		}
		userPeerStakeInfo, ok := userStakeInfo.Peers[peer]
		if !ok {
			userPeerStakeInfo = &UserPeerStakeInfo{}
		}
		userPeerStakeInfo.StakeAmount = userPeerStakeInfo.StakeAmount + amount
		userStakeInfo.Peers[peer] = userPeerStakeInfo

		shardPeerStakeInfo, ok := shardViewInfo.Peers[peer]
		if !ok {
			shardPeerStakeInfo = &PeerViewInfo{}
		}
		shardPeerStakeInfo.WholeStakeAmount = shardPeerStakeInfo.WholeStakeAmount + amount
		shardViewInfo.Peers[peer] = shardPeerStakeInfo
	}
	// update shard stake info and user stake info
	err = setShardViewUserStake(native, id, nextView, user, userStakeInfo)
	if err != nil {
		return fmt.Errorf("userStake: set next view user stake info failed, err: %s", err)
	}
	err = setShardViewInfo(native, id, nextView, shardViewInfo)
	if err != nil {
		return fmt.Errorf("userStake: failed, err: %s", err)
	}

	// update user last stake view num
	err = setUserLastStakeView(native, id, user, nextView)
	if err != nil {
		return fmt.Errorf("userStake: failed, err: %s", err)
	}
	return nil
}

func unfreezeStakeAsset(native *native.NativeService, id types.ShardID, user common.Address, stakeInfo map[string]uint64) error {
	// get view index
	lastStakeView, err := getUserLastStakeView(native, id, user)
	if err != nil {
		return fmt.Errorf("unfreezeStakeAsset: failed, err: %s", err)
	}
	currentView, err := getShardCurrentView(native, id)
	if err != nil {
		return fmt.Errorf("unfreezeStakeAsset: failed, err: %s", err)
	}
	nextView := currentView + 1

	// read user stake info and view stake info
	userStakeInfo, err := getShardViewUserStake(native, id, lastStakeView, user)
	if err != nil {
		return fmt.Errorf("unfreezeStakeAsset: failed, err: %s", err)
	}
	shardViewInfo, err := GetShardViewInfo(native, id, nextView)
	if err != nil {
		return fmt.Errorf("unfreezeStakeAsset: failed, err: %s", err)
	}
	if lastStakeView < currentView {
		// update current user stake info
		err = setShardViewUserStake(native, id, currentView, user, userStakeInfo)
		if err != nil {
			return fmt.Errorf("unfreezeStakeAsset: set current view user stake info failed, err: %s", err)
		}
	} else if lastStakeView > nextView {
		return fmt.Errorf("unfreezeStakeAsset: user last stake view %d and next view %d unmatch",
			lastStakeView, nextView)
	}
	for pubKeyString, amount := range stakeInfo {
		pubKeyData, err := hex.DecodeString(pubKeyString)
		if err != nil {
			return fmt.Errorf("unfreezeStakeAsset: decode pub key %s failed, err: %s", pubKeyString, err)
		}
		peer, err := keypair.DeserializePublicKey(pubKeyData)
		if err != nil {
			return fmt.Errorf("unfreezeStakeAsset: deserialize pub key %s failed, err: %s", pubKeyString, err)
		}
		userPeerStakeInfo, ok := userStakeInfo.Peers[peer]
		if !ok {
			userPeerStakeInfo = &UserPeerStakeInfo{}
		}
		if userPeerStakeInfo.StakeAmount < amount {
			return fmt.Errorf("unfreezeStakeAsset: stake amount %d not enough", userPeerStakeInfo.StakeAmount)
		}
		userPeerStakeInfo.StakeAmount -= amount
		userPeerStakeInfo.UnfreezeAmount += amount
		userStakeInfo.Peers[peer] = userPeerStakeInfo

		shardPeerStakeInfo, ok := shardViewInfo.Peers[peer]
		if !ok {
			shardPeerStakeInfo = &PeerViewInfo{}
		}
		if shardPeerStakeInfo.WholeStakeAmount < amount {
			return fmt.Errorf("unfreezeStakeAsset: whole stake amount %d not enough", shardPeerStakeInfo.WholeStakeAmount)
		}
		shardPeerStakeInfo.WholeStakeAmount -= amount
		shardPeerStakeInfo.WholeUnfreezeAmount += amount
		shardViewInfo.Peers[peer] = shardPeerStakeInfo
	}

	// update next stake info
	err = setShardViewUserStake(native, id, nextView, user, userStakeInfo)
	if err != nil {
		return fmt.Errorf("unfreezeStakeAsset: failed, err: %s", err)
	}
	err = setShardViewInfo(native, id, nextView, shardViewInfo)
	if err != nil {
		return fmt.Errorf("unfreezeStakeAsset: failed, err: %s", err)
	}

	// update user last stake view num
	err = setUserLastStakeView(native, id, user, nextView)
	if err != nil {
		return fmt.Errorf("unfreezeStakeAsset: failed, err: %s", err)
	}
	return nil
}

// return withdraw amount
func withdrawStakeAsset(native *native.NativeService, id types.ShardID, user common.Address) (uint64, error) {
	// get user stake view index
	stakeView, err := getUserLastStakeView(native, id, user)
	if err != nil {
		return 0, fmt.Errorf("withdrawStakeAsset: failed, err: %s", err)
	}
	currentViewIndex, err := getShardCurrentView(native, id)
	if err != nil {
		return 0, fmt.Errorf("withdrawStakeAsset: failed, err: %s", err)
	}
	if stakeView > currentViewIndex {
		stakeView = currentViewIndex
	}
	userStakeInfo, err := getShardViewUserStake(native, id, stakeView, user)
	if err != nil {
		return 0, fmt.Errorf("withdrawStakeAsset: failed, err: %s", err)
	}
	currentViewInfo, err := GetShardViewInfo(native, id, currentViewIndex)
	if err != nil {
		return 0, fmt.Errorf("withdrawStakeAsset: failed, err: %s", err)
	}
	withdrawAmount := uint64(0)
	for peer, userPeerStakeInfo := range userStakeInfo.Peers {
		peerStakeInfo, ok := currentViewInfo.Peers[peer]
		if !ok {
			return 0, fmt.Errorf("withdrawStakeAsset: cannot get current view peer %s stake info",
				hex.EncodeToString(keypair.SerializePublicKey(peer)))
		}
		withdrawAmount += userPeerStakeInfo.UnfreezeAmount
		if peerStakeInfo.WholeUnfreezeAmount < userPeerStakeInfo.UnfreezeAmount {
			return 0, fmt.Errorf("withdrawStakeAsset: whole unfreeze amount %d not enough",
				peerStakeInfo.WholeUnfreezeAmount)
		}
		peerStakeInfo.WholeUnfreezeAmount -= userPeerStakeInfo.UnfreezeAmount
		userPeerStakeInfo.UnfreezeAmount = 0
		userStakeInfo.Peers[peer] = userPeerStakeInfo
		currentViewInfo.Peers[peer] = peerStakeInfo
	}

	err = setShardViewInfo(native, id, currentViewIndex, currentViewInfo)
	if err != nil {
		return 0, fmt.Errorf("withdrawStakeAsset:failed, err: %s", err)
	}
	err = setShardViewUserStake(native, id, currentViewIndex, user, userStakeInfo)
	if err != nil {
		return 0, fmt.Errorf("withdrawStakeAsset:failed, err: %s", err)
	}
	err = setUserLastStakeView(native, id, user, currentViewIndex)
	if err != nil {
		return 0, fmt.Errorf("withdrawStakeAsset:failed, err: %s", err)
	}
	return withdrawAmount, nil
}

// return the amount that user could withdraw
func withdrawFee(native *native.NativeService, shardId types.ShardID, user common.Address) (uint64, error) {
	userWithdrawView, err := getUserLastWithdrawView(native, shardId, user)
	if err != nil {
		return 0, fmt.Errorf("withdrawFee: failed, err: %s", err)
	}
	currentView, err := getShardCurrentView(native, shardId)
	if err != nil {
		return 0, fmt.Errorf("withdrawFee: failed, err: %s", err)
	}
	if currentView == 0 {
		return 0, fmt.Errorf("withdrawFee: init view not support dividends")
	}
	// withdraw view at [userWithdrawView+1, currentView)
	dividends := uint64(0)
	i := userWithdrawView
	count := 0
	supportMul := uint64(100000)
	latestUserStakeInfo := &UserStakeInfo{Peers: make(map[keypair.PublicKey]*UserPeerStakeInfo)}
	lastStakeView, err := getUserLastStakeView(native, shardId, user)
	if err != nil {
		return 0, fmt.Errorf("withdrawFee: failed, err: %s", err)
	}
	if lastStakeView <= userWithdrawView {
		latestUserStakeInfo, err = getShardViewUserStake(native, shardId, lastStakeView, user)
		if err != nil {
			return 0, fmt.Errorf("withdrawFee: get user latest view stake info failed, err: %s", err)
		}
	}
	for ; i < currentView && count < USER_MAX_WITHDRAW_VIEW; i++ {
		userStake, err := getShardViewUserStake(native, shardId, i, user)
		if err != nil {
			return 0, fmt.Errorf("withdrawFee: failed, view %d, err: %s", i, err)
		}
		if !isUserStakePeerEmpty(userStake) {
			if !isUserStakePeerEmpty(latestUserStakeInfo) {
				continue
			} else {
				userStake = latestUserStakeInfo
			}
		}
		viewStake, err := GetShardViewInfo(native, shardId, i)
		if err != nil {
			return 0, fmt.Errorf("withdrawFee: failed, view %d, err: %s", i, err)
		}
		for peer, info := range userStake.Peers {
			peerStakeInfo, ok := viewStake.Peers[peer]
			if !ok {
				return 0, fmt.Errorf("withdrawFee: cannot get view %d peer %s stake info", i,
					hex.EncodeToString(keypair.SerializePublicKey(peer)))
			}
			if peerStakeInfo.FeeBalance == 0 {
				continue
			}
			peerDivide := info.StakeAmount * supportMul * peerStakeInfo.WholeFee / peerStakeInfo.WholeStakeAmount / supportMul
			peerStakeInfo.FeeBalance = peerStakeInfo.FeeBalance - peerDivide
			viewStake.Peers[peer] = peerStakeInfo
			dividends += peerDivide
		}
		err = setShardViewInfo(native, shardId, i, viewStake)
		if err != nil {
			return 0, fmt.Errorf("withdrawFee: failed, view %d, err: %s", i, err)
		}
		count++
		latestUserStakeInfo = userStake
	}
	err = setUserLastWithdrawView(native, shardId, user, currentView-1)
	if err != nil {
		return 0, fmt.Errorf("withdrawFee: failed, view %d, err: %s", i, err)
	}
	err = setShardViewUserStake(native, shardId, i, user, latestUserStakeInfo)
	if err != nil {
		return 0, fmt.Errorf("withdrawFee: failed, view %d, err: %s", i, err)
	}
	return dividends, nil
}

// change peer max authorization and proportion
func changePeerInfo(native *native.NativeService, shardId types.ShardID, peerOwner common.Address, peerPubKey string,
	methodName string, amount uint64) error {
	currentView, err := getShardCurrentView(native, shardId)
	if err != nil {
		return fmt.Errorf("changePeerInfo: failed, err: %s", err)
	}
	nextView := currentView + 1
	nextViewInfo, err := GetShardViewInfo(native, shardId, nextView)
	if err != nil {
		return fmt.Errorf("changePeerInfo: failed, err: %s", err)
	}
	peerInfo, pubKey, err := nextViewInfo.GetPeer(peerPubKey)
	if err != nil {
		return fmt.Errorf("changePeerInfo: failed, err: %s", err)
	}
	if peerInfo.Owner != peerOwner {
		return fmt.Errorf("changePeerInfo: peer owner not match")
	}
	switch methodName {
	case CHANGE_MAX_AUTHORIZATION:
		peerInfo.MaxAuthorization = amount
	case CHANGE_PROPORTION:
		peerInfo.Proportion = amount
	default:
		return fmt.Errorf("changePeerInfo: unsupport change field")
	}
	nextViewInfo.Peers[pubKey] = peerInfo
	if err := setShardViewInfo(native, shardId, nextView, nextViewInfo); err != nil {
		return fmt.Errorf("changePeerInfo: field, err: %s", err)
	}
	return nil
}

func isUserStakePeerEmpty(info *UserStakeInfo) bool {
	if info.Peers == nil || len(info.Peers) == 0 {
		return false
	}
	for _, stakeInfo := range info.Peers {
		if stakeInfo.StakeAmount != 0 || stakeInfo.UnfreezeAmount != 0 {
			return true
		}
	}
	return false
}
