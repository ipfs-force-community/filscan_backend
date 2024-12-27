package chain

import (
	"encoding/json"
	"github.com/shopspring/decimal"
)

var _ json.Marshaler = (*Byte)(nil)
var _ json.Unmarshaler = (*Byte)(nil)

type Byte decimal.Decimal

func (b *Byte) Decimal() decimal.Decimal {
	return decimal.Decimal(*b)
}

func (b *Byte) UnmarshalJSON(bytes []byte) error {
	d := decimal.Decimal{}
	
	err := d.UnmarshalJSON(bytes)
	if err != nil {
		return err
	}
	
	*b = Byte(d)
	
	return err
}

func (b *Byte) MarshalJSON() ([]byte, error) {
	return decimal.Decimal(*b).MarshalJSON()
}
