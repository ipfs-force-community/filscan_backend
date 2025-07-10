package chain

import "github.com/dustin/go-humanize"

type PowerBytes uint64

func (p PowerBytes) Humanize() string {
	return humanize.Bytes(uint64(p))
}
