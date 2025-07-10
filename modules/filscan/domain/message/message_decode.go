package message

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-bitfield"
	"math"
	"strconv"
)

func DecodeMessage(input interface{}) (result []byte, err error) {
	inputParams, ok := input.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected type of params")
	}
	binaryParams, ok := inputParams["$binary"].(map[string]interface{})
	if ok {
		binaryParamsStr, ok := binaryParams["base64"].(string)
		if ok {
			result, err = base64.StdEncoding.DecodeString(binaryParamsStr)
			if err != nil {
				return
			}
		}
	} else {
		dataParamsStr, ok := inputParams["Data"].(string)
		if ok {
			result, err = base64.StdEncoding.DecodeString(dataParamsStr)
			if err != nil {
				return
			}
		}
	}
	return
}

func DecodeBitField(input bitfield.BitField) (result string, err error) {
	var all []uint64
	all, err = input.All(math.MaxUint64)
	if err != nil {
		return
	}
	for i, v := range all {
		if i != len(all)-1 {
			result += strconv.FormatUint(v, 10) + ","
		} else {
			result += strconv.FormatUint(v, 10)
		}
	}
	return
}

func ByteToHex(input []byte) (result string) {
	result = "0x" + hex.EncodeToString(input)
	return
}

func StructToJson(input any) (result string, err error) {
	var marshal []byte
	marshal, err = json.Marshal(input)
	if err != nil {
		return
	}
	result = string(marshal)
	return
}
