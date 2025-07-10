package chain

import (
	"github.com/shopspring/decimal"
	"testing"
)

func TestFil(t *testing.T) {
	
	a := decimal.NewFromFloat(3.126)
	t.Log(a.Round(2))
}
