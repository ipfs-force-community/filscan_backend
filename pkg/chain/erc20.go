package chain

var StdEvent map[string]string

func init() {
	StdEvent = map[string]string{}
	StdEvent["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"] = "Transfer(address indexed from, address indexed to, uint256 value)"
	StdEvent["0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"] = "Approval(address indexed owner, address indexed spender, uint256 value)"
}
