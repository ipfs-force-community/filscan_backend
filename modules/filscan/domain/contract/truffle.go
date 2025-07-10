package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type Truffle struct {
	Compilers Compilers      `json:"compilers"`
	Config    *config.Config `json:"-"`
}

type Compilers struct {
	Solc Solc `json:"solc"`
}

type Solc struct {
	Version    string   `json:"version"`
	Docker     bool     `json:"docker"`
	EvmVersion string   `json:"evmVersion,omitempty"`
	Settings   Settings `json:"settings"`
}

type Settings struct {
	Optimizer Optimizer `json:"optimizer"`
}

type Optimizer struct {
	Enabled bool  `json:"enabled"`
	Runs    int64 `json:"runs"`
}

func (t Truffle) CompileByTruffle(sourceFile []SourceFile) (compiledFile []*ContractInfo, err error) {
	filePath := *t.Config.Solidity.ContractDirectory + strconv.FormatUint(uint64(time.Now().Unix()), 10) + "/"
	err = os.MkdirAll(filePath, os.ModePerm)
	if err != nil {
		return
	}

	cmd := exec.Command("truffle", "init")
	cmd.Dir = filePath
	err = cmd.Run()
	if err != nil {
		return
	}

	err = t.CreateSourceFile(sourceFile, filePath)
	if err != nil {
		return
	}

	err = t.CreateConfigFile(filePath)
	if err != nil {
		return
	}

	cmd = exec.Command("truffle", "compile")
	cmd.Dir = filePath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if stderr.Len() != 0 {
		fmt.Println(stderr)
	}
	err = cmd.Run()
	if err != nil {
		return
	}

	compiledFile, err = t.UnMarshalJsonFile(filePath)
	if err != nil {
		return
	}

	return
}

func (t Truffle) CreateSourceFile(sourceFile []SourceFile, filePath string) (err error) {
	for _, source := range sourceFile {
		fileName := filePath + "contracts/" + source.Name

		var crateFile *os.File
		crateFile, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return
		}

		_, err = io.WriteString(crateFile, source.RawCode)
		if err != nil {
			return
		}
	}
	return
}

func (t Truffle) CreateConfigFile(filePath string) (err error) {
	compilers, err := json.Marshal(t)
	if err != nil {
		return
	}

	configCode := fmt.Sprintf("module.exports = %s;", compilers)
	configFile := filePath + "truffle-config.js"

	var crateConfig *os.File
	crateConfig, err = os.OpenFile(configFile, os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return
	}

	_, err = io.WriteString(crateConfig, configCode)
	if err != nil {
		return
	}
	return
}

func (t Truffle) UnMarshalJsonFile(filePath string) (compiledFile []*ContractInfo, err error) {
	compiledFileDir := filePath + "build/contracts/"
	dirEntries, err := os.ReadDir(compiledFileDir)
	if err != nil {
		return
	}

	for _, dirEntry := range dirEntries {
		compiledFilePath := compiledFileDir + dirEntry.Name()
		var compiledJson []byte
		compiledJson, err = os.ReadFile(compiledFilePath)
		if err != nil {
			return nil, err
		}

		var compileResult *ContractInfo
		err = json.Unmarshal(compiledJson, &compileResult)
		if err != nil {
			log.Fatalf("Failed to unmarshal JSON data: %v", err)
			return
		}
		compiledFile = append(compiledFile, compileResult)
	}

	return
}
