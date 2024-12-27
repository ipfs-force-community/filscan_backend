package erc20

import "sync"

type ContractNameCache struct {
	mpContractIdToTokenName map[string]string
	lk                      sync.RWMutex
}

var contractNameCache ContractNameCache
var contractTokenSymbolCache ContractNameCache
var contractIsPairCache ContractNameCache

func init() {
	contractNameCache = ContractNameCache{
		lk:                      sync.RWMutex{},
		mpContractIdToTokenName: map[string]string{},
	}

	contractTokenSymbolCache = ContractNameCache{
		lk:                      sync.RWMutex{},
		mpContractIdToTokenName: map[string]string{},
	}

	contractDecimalCache = ContractDecimalCache{
		mpContractIdToDecimal: map[string]int{},
		lk:                    sync.RWMutex{},
	}

	contractIsPairCache = ContractNameCache{
		mpContractIdToTokenName: map[string]string{},
		lk:                      sync.RWMutex{},
	}
}

func (c *ContractNameCache) getTokenName(contractID string) *string {
	c.lk.RLock()
	defer c.lk.RUnlock()
	if v, ok := c.mpContractIdToTokenName[contractID]; ok {
		return &v
	}
	return nil
}

func (c *ContractNameCache) setTokenName(contractID, tokenName string) {
	c.lk.Lock()
	defer c.lk.Unlock()
	c.mpContractIdToTokenName[contractID] = tokenName
}

type ContractDecimalCache struct {
	mpContractIdToDecimal map[string]int
	lk                    sync.RWMutex
}

var contractDecimalCache ContractDecimalCache

func (c *ContractDecimalCache) getTokenDecimal(contractID string) *int {
	c.lk.RLock()
	defer c.lk.RUnlock()
	if v, ok := c.mpContractIdToDecimal[contractID]; ok {
		return &v
	}
	return nil
}

func (c *ContractDecimalCache) setTokenDecimal(contractID string, tokenName int) {
	c.lk.Lock()
	defer c.lk.Unlock()
	c.mpContractIdToDecimal[contractID] = tokenName
}
