package fns

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/bundle"
	"io"
)

func getABI(path string) ([]byte, error) {
	f, err := bundle.Templates.Open(path)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = f.Close()
	}()
	
	c, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	
	return c, nil
}
