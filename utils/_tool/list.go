package _tool

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"strings"
)

// RemoveRepByLoop 通过两重循环过滤重复元素
func RemoveRepByLoop[T int64 | string](input []T) (result []T) {
	for i := range input {
		flag := true
		for j := range result {
			if input[i] == result[j] {
				flag = false // 存在重复元素，标识为false
				break
			}
		}
		if flag { // 标识为false，不添加进结果
			result = append(result, input[i])
		}
	}
	return result
}

func MoveToTopForBlockBasic(basic []*filscan.BlockBasic, sourceIndex int, targetIndex int) []*filscan.BlockBasic {
	indexToRemove := sourceIndex
	indexWhereToInsert := targetIndex

	slice := basic

	val := slice[indexToRemove]

	slice = append(slice[:indexToRemove], slice[indexToRemove+1:]...)

	newSlice := make([]*filscan.BlockBasic, indexWhereToInsert+1)
	copy(newSlice, slice[:indexWhereToInsert])
	newSlice[indexWhereToInsert] = val

	slice = append(newSlice, slice[indexWhereToInsert:]...)
	return slice
}

func MoveToTopForGroupInfo(basic []*pro.GroupInfo, sourceIndex int, targetIndex int) []*pro.GroupInfo {
	indexToRemove := sourceIndex
	indexWhereToInsert := targetIndex

	slice := basic

	val := slice[indexToRemove]

	slice = append(slice[:indexToRemove], slice[indexToRemove+1:]...)

	newSlice := make([]*pro.GroupInfo, indexWhereToInsert+1)
	copy(newSlice, slice[:indexWhereToInsert])
	newSlice[indexWhereToInsert] = val

	slice = append(newSlice, slice[indexWhereToInsert:]...)
	return slice
}

func RemoveRepForActor(slc []*londobell.ActorState) []*londobell.ActorState {
	var result []*londobell.ActorState // 存放结果
	for i := range slc {
		flag := true
		for j := range result {
			if slc[i].ActorID == result[j].ActorID || slc[i].ActorID == "" {
				flag = false // 存在重复元素，标识为false
				break
			}
		}
		if flag { // 标识为false，不添加进结果
			result = append(result, slc[i])
		}
	}
	return result
}

func RemoveEmptyStrings(arr []string) []chain.SmartAddress {
	result := make([]chain.SmartAddress, 0, len(arr))
	for _, str := range arr {
		trimmed := strings.TrimSpace(str)
		if trimmed != "" {
			result = append(result, chain.SmartAddress(trimmed))
		}
	}
	return result
}
