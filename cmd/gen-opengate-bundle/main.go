package main

import (
	"github.com/gozelle/vfs"
	"log"
	"net/http"
)

func main() {
	err := vfs.Generate(http.Dir("modules/fevm/protocol"), vfs.Options{
		Filename:        "modules/fevm/bundle/bundle.prod.go",
		PackageName:     "bundle",
		BuildTags:       "bundle",
		VariableName:    "Templates",
		VariableComment: "abi 文件",
	})
	if err != nil {
		log.Panicf("generate bundle error: %s", err)
	}
}
