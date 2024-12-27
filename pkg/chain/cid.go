package chain

import (
	"github.com/ipfs/go-cid"
)

func CompareCidsEquals(a, b []cid.Cid) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v.String() != b[i].String() {
			return false
		}
	}
	return true
}

func CompareStringsCidsEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func CidsToStrings(cids []cid.Cid) []string {
	var r []string
	for _, v := range cids {
		r = append(r, v.String())
	}
	return r
}
