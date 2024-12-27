package chain

import "github.com/shopspring/decimal"

var (
	AttoFilPrecision = decimal.NewFromFloat(1e18)
	nanoFilPrecision = decimal.NewFromFloat(1e9)
)

func MustBuildNanoFil(v string) AttoFil {
	d, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}
	return AttoFil(d)
}

type Fil = decimal.Decimal

func NewNanoFil(v decimal.Decimal) AttoFil {
	return AttoFil(v)
}

type AttoFil decimal.Decimal

func (n *AttoFil) UnmarshalJSON(bytes []byte) error {
	d := decimal.Decimal{}
	err := d.UnmarshalJSON(bytes)
	if err != nil {

		return err
	}
	*n = AttoFil(d)
	return nil
}

func (n *AttoFil) MarshalJSON() ([]byte, error) {
	return decimal.Decimal(*n).MarshalJSON()
}

func (n *AttoFil) Decimal() decimal.Decimal {
	return decimal.Decimal(*n)
}

func (n *AttoFil) Fil() decimal.Decimal {
	return decimal.Decimal(*n).Div(AttoFilPrecision)
}

func (n *AttoFil) FilRound(round int32) decimal.Decimal {
	return n.Fil().Round(round)
}

func (n *AttoFil) NanoFil() decimal.Decimal {
	return decimal.Decimal(*n).Div(nanoFilPrecision)
}

func (n *AttoFil) Scan(v interface{}) error {
	vv := decimal.Decimal{}
	err := vv.Scan(v)
	if err != nil {
		return err
	}
	*n = AttoFil(vv)
	return nil
}

func (n *AttoFil) String() string {
	return decimal.Decimal(*n).String()
}
