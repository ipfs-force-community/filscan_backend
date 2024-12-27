package chain

import (
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/filecoin-project/go-address"
)

func init() {
	address.CurrentNetwork = address.Mainnet
}

func RegisterNet(testNet bool) {
	if testNet {
		address.CurrentNetwork = address.Testnet
	} else {
		address.CurrentNetwork = address.Mainnet
	}
}

var (
	UndefAddressString = address.UndefAddressString // <empty>
	RewardActorAddress address.Address
	PowerActorAddress  address.Address
	MarketActorAddress address.Address
)

func init() {
	var err error
	RewardActorAddress, err = address.NewIDAddress(2)
	if err != nil {
		panic(err)
	}
	PowerActorAddress, err = address.NewIDAddress(4)
	if err != nil {
		panic(err)
	}
	MarketActorAddress, err = address.NewIDAddress(5)
	if err != nil {
		panic(err)
	}
}

// AutoWrapPureAddress 如果传入的地址是一个合法的地址则原值返回
// 如果传入的是一个没有前缀的地址，则包装前缀返回
//func AutoWrapPureAddress(addr string) string {
//	if _, err := address.NewFromString(addr); err == nil {
//		return addr
//	}
//	switch address.CurrentNetwork {
//	case address.Mainnet:
//		return address.MainnetPrefix + addr
//	default:
//		return address.TestnetPrefix + addr
//	}
//}

// IsPureActorID 是否为不带前缀的 ActorID
func IsPureActorID(addr string) bool {
	if len(addr) == 0 {
		return false
	}
	return string(addr[0]) == "0"
}

// SmartAddress 智能地址
// 因为 londobell agg 请求和响应的地址均不带前缀，而 londobell adapter 的请求返回地址均带前缀，故用此类型来区分。
type SmartAddress string

func (s SmartAddress) Value() (driver.Value, error) {
	err := s.Valid()
	if err != nil {
		return nil, err
	}
	return s.Address(), nil
}

// Valid 此方法将会验证地址的正确性
func (s SmartAddress) Valid() error {

	if len(s) == 0 {
		return fmt.Errorf("emtpy string")
	}

	_, err := address.NewFromString(s.Address())
	if err != nil {
		return err
	}
	return nil
}

// Address 此方法返回带前缀的地址，不判断地址的正确性
// 注：<empty> 地址直接返回
func (s SmartAddress) Address() string {
	addr := string(s)
	if addr == "" {
		return ""
	}
	if addr == UndefAddressString {
		return addr
	}
	if !strings.HasPrefix(addr, "f") && !strings.HasPrefix(addr, "t") {
		switch address.CurrentNetwork {
		case address.Mainnet:
			addr = address.MainnetPrefix + addr
		default:
			addr = address.TestnetPrefix + addr
		}
	}
	return addr
}

// CrudeAddress 该方法用于提供 londobell agg 所需要的不带前缀的简陋的地址
func (s SmartAddress) CrudeAddress() string {
	addr := s.Address()
	if len(addr) == 0 {
		return ""
	}
	return addr[1:]
}

func (s SmartAddress) IsID() bool {
	if err := s.Valid(); err != nil {
		return false
	}
	if string(s.Address()[1]) == "0" {
		return true
	} else {
		return false
	}
}

func (s SmartAddress) IsAddress() bool {
	if err := s.Valid(); err != nil {
		return false
	}
	if string(s.Address()[1]) != "0" {
		return true
	} else {
		return false
	}
}

func (s SmartAddress) IsValid() bool {
	if err := s.Valid(); err == nil {
		return true
	}
	return false
}

func (s SmartAddress) IsEmpty() bool {
	if s.Address() == UndefAddressString {
		return true
	}
	return false
}

func (s SmartAddress) IsEthAddress() bool {
	return strings.HasPrefix(string(s), "0x")
}

func (s SmartAddress) IsContract() bool {
	if err := s.Valid(); err != nil {
		return false
	}
	if string(s.Address()[1]) == "4" {
		return true
	} else {
		return false
	}
}

func TrimHexAddress(addr string) string {
	a := &big.Int{}
	a.SetString(strings.TrimPrefix(addr, "0x"), 16)
	if a.Int64() == 0 {
		return "0x0000000000000000000000000000000000000000"
	}
	return fmt.Sprintf("0x%s", hex.EncodeToString(a.Bytes()))
}
