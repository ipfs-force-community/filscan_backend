package fevm

type ABIDecoderAPI interface {
	ContractAPI
	FNSAPI
}

type DecodeMethodInputReply struct {
	Name    string
	RawName string
	Values  []any
}

type DecodeEventInputReply struct {
	Name    string
	RawName string
	Values  []any
}

type ABISignature struct {
	Type string
	Name string
	Id   string
	Raw  string
}

type ContractParam struct {
	Type  string
	Value interface{}
}
