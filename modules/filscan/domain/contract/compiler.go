package contract

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/common/compiler"
	json "github.com/nikkolasg/hexjson"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Compiler struct {
	Solidity   *Solidity
	Optimize   *Optimize
	Config     *config.Config
	IsMetaData bool
	mutex      sync.Mutex
}

func (c *Compiler) ChangeVersion(solcSelectPath, targetVersion string) (err error) {
	err = c.Solidity.ChangeVersion(solcSelectPath, targetVersion)
	if err != nil {
		return
	}
	changedSolidity, err := NewSolidity(c.Solidity.Path)
	c.Solidity = changedSolidity
	if err != nil {
		return
	}
	err = changedSolidity.String()
	return
}

type SourceFile struct {
	Name    string
	RawCode string
}

type StandardFile struct { //先不用
	Name     string
	StdInput *StandardInput
}

// Optimize enable、runs都为可选字段，未来可能被抛弃，好在默认为false和0，但是有其他优化参数情况下，不能考虑enable的默认值
type Optimize struct {
	Enabled bool              `json:"enabled"`
	Runs    int64             `json:"runs"`
	Details *OptimizerDetails `json:"details,omitempty"`
}

type OptimizerDetails struct {
	Peephole          *bool `json:"peephole,omitempty"`
	Inliner           *bool `json:"inliner,omitempty"`
	JumpdestRemover   *bool `json:"jumpdestRemover,omitempty"`
	OrderLiterals     *bool `json:"orderLiterals,omitempty"`
	Deduplicate       *bool `json:"deduplicate,omitempty"`
	Cse               *bool `json:"cse,omitempty"`
	ConstantOptimizer *bool `json:"constantOptimizer,omitempty"`
	Yul               *bool `json:"yul,omitempty"`
	YulDetails        *struct {
		StackAllocation *bool   `json:"stackAllocation,omitempty"`
		OptimizerSteps  *string `json:"optimizerSteps,omitempty"`
	} `json:"yulDetails,omitempty"`
}

type Setting struct {
	CompilationTarget map[string]string `json:"compilationTarget,omitempty"`
	EvmVersion        string            `json:"evmVersion,omitempty"`
	Libraries         interface{}       `json:"libraries,omitempty"`
	Optimizer         *Optimize         `json:"optimizer"`
	Remappings        []interface{}     `json:"remappings,omitempty"`
	OutputSelection   interface{}       `json:"outputSelection,omitempty"`
	ViaIR             bool              `json:"viaIR,omitempty"`
	Metadata          *MetadataSetting  `json:"metadata"`
}

func GetDefaultOutputSelection() (interface{}, error) {
	var defaultOutputSelection interface{}
	defaultModel := "{\n            \"*\": {\n                \"*\": [\n                    \"abi\",\n                    \"evm.bytecode\",\n                    \"evm.deployedBytecode\",\n                    \"evm.methodIdentifiers\",\n                    \"metadata\",\n                    \"devdoc\",\n                    \"userdoc\",\n                    \"storageLayout\",\n                    \"evm.gasEstimates\"\n                ],\n                \"\": [\n                    \"ast\"\n                ]\n            }\n        }"
	err := json.Unmarshal([]byte(defaultModel), &defaultOutputSelection)
	return defaultOutputSelection, err
}

type MetadataSetting struct {
	UseLiteralContent bool    `json:"useLiteralContent,omitempty"`
	BytecodeHash      *string `json:"bytecodeHash,omitempty"`
}

type MetaData struct {
	Compiler struct {
		Version string `json:"version"`
	} `json:"compiler"`
	Language string `json:"language"`
	Output   struct {
		Abi     interface{} `json:"abi"`
		Devdoc  interface{} `json:"devdoc"`
		Userdoc interface{} `json:"userdoc"`
	} `json:"output"`
	Settings *Setting          `json:"settings"`
	Sources  map[string]Source `json:"sources"`
	Version  int               `json:"version"`
}

type Source struct {
	Keccak256 string   `json:"keccak256,omitempty"`
	License   string   `json:"license,omitempty"`
	Urls      []string `json:"urls,omitempty"`
	Content   *string  `json:"content,omitempty"`
}

type HardhatInput struct {
	Input       *StandardInput   `json:"input"`
	SolcVersion string           `json:"solcVersion"`
	Ouput       *BuildInfoOutput `json:"output"`
}

type StandardInput struct {
	Language string            `json:"language,omitempty"`
	Sources  map[string]Source `json:"sources,omitempty"`
	Settings *Setting          `json:"settings,omitempty"`
}

type BuildInfoOutput struct {
	Contracts map[string]map[string]BuildInfoOutputMetadata `json:"contracts,omitempty"`
}

type BuildInfoOutputMetadata struct {
	MetaDataString string `json:"metadata,omitempty"`
}

type SolidityFile map[string]*struct { //todo 改了指针，看看有没有问题
	Bin, Ir, Metadata string
	Abi               interface{}
	Devdoc            interface{}
	Userdoc           interface{}
	Evm               struct {
		Bytecode struct {
			FunctionDebugData struct {
			} `json:"functionDebugData"`
			GeneratedSources []interface{} `json:"generatedSources"`
			LinkReferences   struct {
			} `json:"linkReferences"`
			Object    string `json:"object"`
			Opcodes   string `json:"opcodes"`
			SourceMap string `json:"sourceMap"`
		} `json:"bytecode"`
		DeployedBytecode struct {
			FunctionDebugData struct {
			} `json:"functionDebugData"`
			GeneratedSources    []interface{} `json:"generatedSources"`
			ImmutableReferences struct {
			} `json:"immutableReferences"`
			LinkReferences struct {
			} `json:"linkReferences"`
			Object    string `json:"object"`
			Opcodes   string `json:"opcodes"`
			SourceMap string `json:"sourceMap"`
		} `json:"deployedBytecode"`
		GasEstimates      interface{} `json:"gasEstimates"`
		MethodIdentifiers interface{} `json:"methodIdentifiers"`
	} `json:"evm"`
}

type StandardOutput struct {
	Errors    []map[string]interface{} `json:"errors,omitempty"`
	Sources   map[string]interface{}   `json:"sources"`
	Contracts map[string]SolidityFile  `json:"contracts,omitempty"`
}

func (c *Compiler) CreatedSourceFile(sourceFile SourceFile) (err error) {
	dir := filepath.Dir(sourceFile.Name) //这就是获取在服务器中存放合约的位置（自定义）
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}
	var file *os.File
	file, err = os.Create(sourceFile.Name) //保存文件到哪儿 这儿还有目录
	if err != nil {
		return
	}
	_, err = io.WriteString(file, sourceFile.RawCode)
	if err != nil {
		return
	}
	return
}

func (c *Compiler) CompileStandardJson(file *SourceFile, fileDir string) (compiledFiles []*CompiledFile, err error) {
	res, err := c.CompileJson(*file, fileDir)
	if err != nil {
		return
	}
	for _, val := range res.Errors {
		if val["type"] == "Warning" || val["type"] == "Info" {
			continue
		} else {
			marshal, err1 := json.Marshal(res.Errors)
			if err1 != nil {
				return compiledFiles, err1
			}
			err = fmt.Errorf(string(marshal))
			return
		}
	}
	var abi []byte
	stdInput := &StandardInput{}
	err = json.Unmarshal([]byte(file.RawCode), stdInput)
	if err != nil {
		return
	}
	for solidityFileName, solidityFile := range res.Contracts {
		//if strings.Split(solidityFileName, "/")[0] == "@openzeppelin" {
		//	continue
		//}
		for contractName, contract := range solidityFile {
			abi, err = json.Marshal(contract.Abi)
			if err != nil {
				return
			}
			compiledFiles = append(compiledFiles, &CompiledFile{
				ContractName: contractName,
				ByteCode:     "0x" + contract.Evm.Bytecode.Object,
				ABI:          string(abi),
				ContractFile: SourceFile{
					Name:    solidityFileName,
					RawCode: *stdInput.Sources[solidityFileName].Content,
				},
			})
		}
	}
	return
}

func (c *Compiler) CompileStandardJsonWithTarget(file *SourceFile, fileDir string, target string) (compiledFiles []*CompiledFile, err error) {
	res, err := c.CompileJson(*file, fileDir)
	if err != nil {
		return
	}
	for _, val := range res.Errors {
		if val["type"] == "Warning" || val["type"] == "Info" {
			continue
		} else {
			marshal, err1 := json.Marshal(res.Errors)
			if err1 != nil {
				return compiledFiles, err1
			}
			err = fmt.Errorf(string(marshal))
			return
		}
	}
	var abi []byte
	stdInput := &StandardInput{}
	err = json.Unmarshal([]byte(file.RawCode), stdInput)
	if err != nil {
		return
	}
	//todo 为了以后的扩展，这些其他非主合约的编译结果可以不要，但是得保存相关的源码。方便显示
	//todo 这里的target没意义了，放到外面去，这里所有的都要保存
	for solidityFileName, solidityFile := range res.Contracts {
		//if strings.Split(solidityFileName, "/")[0] == "@openzeppelin" {
		//	continue
		//}
		if solidityFile[target] != nil {
			abi, err = json.Marshal(solidityFile[target].Abi)
			if err != nil {
				return
			}
			compiledFiles = append(compiledFiles, &CompiledFile{
				ContractName: target,
				ByteCode:     "0x" + solidityFile[target].Evm.Bytecode.Object,
				ABI:          string(abi),
				ContractFile: SourceFile{
					Name:    solidityFileName,
					RawCode: *stdInput.Sources[solidityFileName].Content,
				},
				IsMainContract: true,
			})
			break
		}
	}
	return
}

func (c *Compiler) CompileSourceFile(sourceFile []SourceFile, fileDir string) (compiledFiles []*CompiledFile, err error) {
	var savedSourceFile []SourceFile //主要是将目录地址给去掉，但在该函数中利用了sourcefile中的目录
	for _, file := range sourceFile {
		err = c.CreatedSourceFile(file)
		if err != nil {
			return
		}
		file.Name = strings.ReplaceAll(file.Name, fileDir, "") // 将文件名中的目录地址去掉
		savedSourceFile = append(savedSourceFile, file)
	}

	compiledFiles, err = c.CompileSavedFiles(savedSourceFile, fileDir)
	if err != nil {
		return
	}

	return
}

func (c *Compiler) CreatedMateDataFile(mateDataFile SourceFile) (savedFile MetaData, err error) {
	err = c.CreatedSourceFile(mateDataFile)
	if err != nil {
		return
	}

	err = json.Unmarshal([]byte(mateDataFile.RawCode), &savedFile)
	if err != nil {
		return
	}
	return
}

func (c *Compiler) CompileWithMetaData(sourceFile []SourceFile, metaData MetaData, fileDir string) (compiledFiles []*CompiledFile, err error) {
	input := StandardInput{
		Language: "Solidity",
		Sources:  make(map[string]Source),
		Settings: &Setting{},
	}
	//根据Metadata里的信息填充standard-input的source
	for key := range metaData.Sources {
		if metaData.Sources[key].Content != nil {
			input.Sources[key] = Source{Content: metaData.Sources[key].Content}
			continue
		}
		for _, file := range sourceFile {
			splitKey := strings.Split(key, "/")
			splitSource := strings.Split(file.Name, "/")
			if splitSource[len(splitSource)-1] == splitKey[len(splitKey)-1] {
				// todo 如何更好的进行匹配，不用双重循环，但是空间换时间
				rawcode := file.RawCode
				input.Sources[key] = Source{Content: &rawcode}
				break
			}
		}
		_, ok := input.Sources[key]
		if !ok {
			err = fmt.Errorf("note: The files required for compilation have not been uploaded yet, please upload the files or check whether the file name has been modified")
			return
		}
	}
	outputSelection, err := GetDefaultOutputSelection()
	if err != nil {
		return
	}
	input.Settings = &Setting{
		CompilationTarget: nil,
		EvmVersion:        metaData.Settings.EvmVersion,
		Optimizer: &Optimize{
			Enabled: metaData.Settings.Optimizer.Enabled,
			Runs:    metaData.Settings.Optimizer.Runs,
			Details: metaData.Settings.Optimizer.Details,
		},
		OutputSelection: outputSelection,
		ViaIR:           metaData.Settings.ViaIR,
		Metadata: &MetadataSetting{
			UseLiteralContent: true,
		},
	}
	var rawcode []byte
	rawcode, err = json.Marshal(input)
	if err != nil {
		return
	}
	inputFile := SourceFile{
		Name:    fileDir + strconv.Itoa(int(time.Now().Unix())) + "standardInput.json",
		RawCode: string(rawcode),
	}
	//var target string
	//for _, v := range metaData.Settings.CompilationTarget {
	//	target = v
	//}
	//compiledFiles, err = c.CompileStandardJsonWithTarget(&inputFile, fileDir, target)
	compiledFiles, err = c.CompileStandardJson(&inputFile, fileDir)
	//compiledFiles, err = c.CompileSavedFiles(savedSourceFile, fileDir)
	if err != nil { //换另一种方式编译试试
		if strings.Index(err.Error(), "JSONError") != -1 {
			var savedSourceFile []SourceFile //先把文件保存在本地，再去编译
			for _, file := range sourceFile {
				for key := range metaData.Sources {
					splitKey := strings.Split(key, "/")
					splitSource := strings.Split(file.Name, "/")
					if splitSource[len(splitSource)-1] == splitKey[len(splitKey)-1] {
						file.Name = fileDir + key
					}
				}
				err = c.CreatedSourceFile(file)
				if err != nil {
					return
				}
				savedSourceFile = append(savedSourceFile, file)
			}
			compiledFiles, err = c.CompileSavedFiles(savedSourceFile, fileDir)
			if err != nil {
				return
			}
		}
	}

	return
}

func (c *Compiler) CompileSavedFiles(savedFiles []SourceFile, fileDir string) (compiledFiles []*CompiledFile, err error) {
	allCompileResult := make(map[string]*compiler.Contract)
	for _, file := range savedFiles {
		var compileResult map[string]*compiler.Contract
		compileResult, err = c.Compile(file, fileDir) //合约名字 -》 合约内容结构体  一个map
		if err != nil {
			return
		}
		for key, value := range compileResult {
			allCompileResult[key] = value
		}
	}
	for key, value := range allCompileResult {
		splitKey := strings.Split(key, ":")       //编译后的合约是文件名 + 合约名 如Resolver_flattened.sol:IResolver
		contractName := splitKey[len(splitKey)-1] // 合约名
		compiledFileName := splitKey[0]           //文件名
		var compiledFileCode string
		for _, savedFile := range savedFiles { //找到当前编译码的源码在哪里
			splitFileName := strings.Split(compiledFileName, "/")[len(strings.Split(compiledFileName, "/"))-1]
			savedFileName := strings.Split(savedFile.Name, "/")[len(strings.Split(savedFile.Name, "/"))-1]
			if splitFileName == savedFileName {
				compiledFileCode = savedFile.RawCode
			}
		}
		var abi []byte
		abi, err = json.Marshal(value.Info.AbiDefinition)
		if err != nil {
			return
		}
		compiledFiles = append(compiledFiles, &CompiledFile{
			ContractName: contractName,
			ByteCode:     value.Code,
			ABI:          string(abi),
			ContractFile: SourceFile{
				Name:    compiledFileName,
				RawCode: compiledFileCode,
			},
		})
	}

	return
}
func (c *Compiler) CompileJson(sourceFile SourceFile, fileDir string) (compileResult StandardOutput, err error) {
	if c.Solidity.Path == "" {
		c.Solidity.Path = "solc"
	}
	err = c.CreatedSourceFile(sourceFile)
	if err != nil {
		return
	}
	args := []string{
		"--standard-json",
		sourceFile.Name,
	}
	cmd := exec.Command(c.Solidity.Path, args...) //调用命令真实去编译
	fmt.Println(cmd)
	var stderr, stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err = cmd.Run(); err != nil {
		return
	}
	err = json.Unmarshal(stdout.Bytes(), &compileResult)
	return
}

func (c *Compiler) Compile(sourceFile SourceFile, fileDir string) (compileResult map[string]*compiler.Contract, err error) {
	if c.Solidity.Path == "" {
		c.Solidity.Path = "solc"
	}
	args := c.Solidity.MakeArgs(sourceFile.Name, c.Optimize, c.IsMetaData)
	cmd := exec.Command(c.Solidity.Path, args...)
	cmd.Dir = fileDir
	fmt.Println(cmd)
	var stderr, stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err = cmd.Run(); err != nil {
		err = fmt.Errorf("%s", stderr.String())
		return
	}
	// 这里是直接将结果转换接受solc ——combined-output 运行的直接输出，提供的源代码、语言和编译器版本，以及编译器选项都被传递到Contract结构中
	// 期望solc输出包含ABI、源映射、用户文档和开发文档。如果JSON格式错误或缺少数据，或者JSON中嵌入的JSON 格式错误，则返回错误。
	compileResult, err = compiler.ParseCombinedJSON(stdout.Bytes(), sourceFile.RawCode, c.Solidity.Version,
		c.Solidity.Version, strings.Join(c.Solidity.MakeArgs(sourceFile.Name, c.Optimize, c.IsMetaData), " "))
	if err != nil {
		return
	}
	return
}
