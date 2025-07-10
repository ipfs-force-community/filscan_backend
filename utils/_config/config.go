package _config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/gozelle/fs"
)

func UnmarshalConfigFile(file string, target any) (err error) {
	
	if !fs.IsFile(file) {
		err = fmt.Errorf("%s is not file", file)
		return
	}
	
	d, err := fs.Read(file)
	if err != nil {
		return
	}
	
	err = toml.Unmarshal(d, target)
	if err != nil {
		return
	}
	
	return
}
