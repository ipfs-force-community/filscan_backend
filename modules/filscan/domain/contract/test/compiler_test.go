package test

import (
	//"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/gozelle/spew"
	"github.com/gozelle/testify/assert"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/biz/browser"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/contract"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"time"
)

/*
用于保存爬取并转换链上数据的相关数据结构
*/
type FilfoxContract struct {
	Verified     bool   `json:"verified"`
	InitCode     string `json:"initCode"`
	ContractName string `json:"contractName"`
	Abi          string `json:"abi"`
	Language     string `json:"language"`
	Compiler     string `json:"compiler"`
	Parameters   string `json:"parameters"`
	License      string `json:"license"`
	Optimize     bool   `json:"optimize"`
	OptimizeRuns int64  `json:"optimizeRuns"`
	OptDetails   string `json:"optimizerDetails,omitempty"`
	EvmVersion   string `json:"evmVersion"`
	ViaIR        bool   `json:"viaIR"`
	SourceFiles  []struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	} `json:"sourceFiles"`
	ProxyImpl string `json:"proxyImpl"`
}

type StarboardRes struct {
	Data struct {
		Address         string       `json:"address"`
		ContractName    string       `json:"contract_name"`
		CompilerVersion string       `json:"compiler_version"`
		Metadata        string       `json:"metadata"`
		SourceCodes     []SourceCode `json:"source_codes"`
	} `json:"data"`
}

type SourceCode struct {
	FileName string `json:"file_name"`
	Code     string `json:"code"`
}

type StarboardContract struct {
	Addr            string                `json:"addr,omitempty"`
	ContractName    string                `json:"contractName,omitempty"`
	MetaData        *contract.MetaData    `json:"metaData,omitempty"`
	CompilerVersion string                `json:"compilerVersion,omitempty"`
	SourceFiles     []contract.SourceFile `json:"sourceFiles,omitempty"`
}

type CsvContent struct {
	address string
	content string
}

/*
读取文件及保存到文件
*/
func readFileToBytes(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			return
		}
	}(file)

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func readCSV(filePath string) ([][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			return
		}
	}(file)
	reader := csv.NewReader(file)
	reader.Comma = ',' // 设置字段分隔符，默认为逗号
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	return records, nil
}

func saveJson(s string) (err error) {
	filename := strconv.Itoa(int(time.Now().Unix())) + "starboardContract.json"
	file, err := os.Create(filename)
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			return
		}
	}(file)
	if err != nil {
		return
	}
	_, err = io.WriteString(file, s)
	return err
}

/*
在测试前做一些初始化工作
*/
func initalAgg() (err error) {
	// 读取配置文件
	conf := &config.Config{}
	err = _config.UnmarshalConfigFile("/Users/jinshi/Downloads/filscan-backend/configs/local.toml", conf)
	if err != nil {
		log.Fatal(err)
	}
	spew.Json(conf)

	agg, err := injector.NewLondobellAgg(conf)
	v = browser.NewVerifyContract(agg, nil, nil, conf)
	if err != nil {
		return
	}
	return
}

func initStarboardRes() (err error) {
	starboardVerifiedContracts := "./1691657825starboardContract.json"
	addrCode1237 := "/Users/jinshi/Downloads/filscan-backend/modules/filscan/domain/contract/test/addr_code1237.json"
	addrStarboardContractMap = make(map[string]*StarboardContract)
	addrInitcode1237 = make(map[string]string)
	bytes, err := readFileToBytes(starboardVerifiedContracts)
	if err != nil {
		fmt.Println(err)
		return
	}
	bytes2, err := readFileToBytes(addrCode1237)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = json.Unmarshal(bytes, &addrStarboardContractMap)
	if err != nil {
		return
	}
	return json.Unmarshal(bytes2, &addrInitcode1237)
	//var rawcode []byte
	//rawcode, err = json.Marshal(input)
	//sfile := SourceFile{
	//	Name:    filfoxdir + strconv.Itoa(i) + contract.ContractName + "_StandardJson.json",
	//	RawCode: string(rawcode),
	//}
	//StandInputJsonFiles = append(StandInputJsonFiles, &sfile)
	//initcodes = append(initcodes, "0x"+contract.InitCode)
	//contractNames = append(contractNames, contract.ContractName)

	//solcVersion = append(solcVersion, targetVersion)
}

func initFilfoxRes() (err error) {
	filfoxVerifiedContracts := "/Users/jinshi/Documents/test/filfox_verified_contracts_detail.json"
	var contracts []FilfoxContract
	bytes, err := readFileToBytes(filfoxVerifiedContracts)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = json.Unmarshal(bytes, &contracts)
	if err != nil {
		return
	}
	outputSelection, err := contract.GetDefaultOutputSelection()
	if err != nil {
		return
	}
	for i, con := range contracts {
		input := contract.StandardInput{
			Language: "Solidity",
			Sources:  make(map[string]contract.Source),
			Settings: &contract.Setting{},
		}
		for _, file := range con.SourceFiles {
			rawcode := file.Content
			input.Sources[file.Name] = contract.Source{Content: &rawcode}
		}
		var optDetails contract.OptimizerDetails
		var detailPointer *contract.OptimizerDetails
		if con.OptDetails != "" {
			err = json.Unmarshal([]byte(con.OptDetails), &optDetails)
			detailPointer = &optDetails
			if err != nil {
				return
			}
		}
		evmVersion := ""
		if con.EvmVersion != "default" {
			evmVersion = con.EvmVersion
		}
		input.Settings = &contract.Setting{
			CompilationTarget: nil,
			EvmVersion:        evmVersion,
			Optimizer: &contract.Optimize{
				Enabled: con.Optimize,
				Runs:    con.OptimizeRuns,
				Details: detailPointer,
			},
			OutputSelection: outputSelection,
			ViaIR:           con.ViaIR,
			Metadata: &contract.MetadataSetting{
				UseLiteralContent: true,
			},
		}
		StandInputJsons = append(StandInputJsons, input)
		var rawcode []byte
		rawcode, err = json.Marshal(input)
		if err != nil {
			return
		}
		sfile := contract.SourceFile{
			Name:    filfoxdir + strconv.Itoa(i) + con.ContractName + "_StandardJson.json",
			RawCode: string(rawcode),
		}
		StandInputJsonFiles = append(StandInputJsonFiles, &sfile)
		initcodes = append(initcodes, "0x"+con.InitCode)
		contractNames = append(contractNames, con.ContractName+strconv.Itoa(i))
		var targetVersion string
		matches := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindStringSubmatch(con.Compiler)
		if len(matches) != 1 {
			err = fmt.Errorf("can't parse solc version %q", con.Compiler)
			return
		} else {
			targetVersion = matches[0]
		}
		solcVersion = append(solcVersion, targetVersion)
	}
	return
}

/*
获取所有的合约代码、Metadata等其他数据
*/
func getStarboardInfo() {
	type A struct {
		Address string `json:"address"`
	}
	f, err := os.Open("/Users/jinshi/Documents/test/startboard.json")
	if err != nil {
		panic(err)
	}
	bs, err := io.ReadAll(f)
	if err != nil {
		panic(err)
	}
	res := []A{}
	err = json.Unmarshal(bs, &res)
	if err != nil {
		panic(err)
	}
	ff, err := os.Create("/Users/jinshi/Documents/test/starboard_verified_contracts.json")
	if err != nil {
		panic(err)
	}
	for i := range res {
		for {
			client := &http.Client{}
			req, _ := http.NewRequest("GET", fmt.Sprintf("https://fvm-api.starboard.ventures/api/v1/contract/%s", res[i].Address), nil)

			res, err := client.Do(req)
			if err != nil {
				fmt.Println(err)
				time.Sleep(time.Minute)
				continue
			}
			rb, err := io.ReadAll(res.Body)
			if err != nil {
				fmt.Println(err)
				time.Sleep(time.Minute)
				continue
			}
			_, err = ff.WriteString(string(rb))
			if err != nil {
				fmt.Println(err)
				time.Sleep(time.Minute)
				continue
			}
			_, err = ff.WriteString("\n")
			if err != nil {
				fmt.Println(err)
				time.Sleep(time.Minute)
				continue
			}
			time.Sleep(time.Second)
			break
		}
	}
}

func fetchUrl(url string) (res string, err error) {
	// 发起HTTP GET请求
	response, err := http.Get(url)
	if err != nil {
		fmt.Printf("HTTP GET请求失败：%s\n", err)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(response.Body)

	// 读取响应的内容
	body, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("读取响应内容失败：%s\n", err)
		return
	}
	return string(body), nil
}

// 第一次通过json获取和整理starboard上所有的数据，只需要运行一次（耗时）
func fetchAllStarBoardContract() (err error) {
	starboardVerifiedContracts := "/Users/jinshi/Documents/test/starboard_verified_contracts.json"
	var contracts []StarboardRes
	bytes, err := readFileToBytes(starboardVerifiedContracts)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = json.Unmarshal(bytes, &contracts)
	if err != nil {
		return
	}
	addrStarboardContractMap = make(map[string]*StarboardContract)
	for _, con := range contracts {
		addr := con.Data.Address
		var metaData contract.MetaData
		err = json.Unmarshal([]byte(con.Data.Metadata), &metaData)
		if err != nil {
			return
		}
		//获取并校验版本
		var targetVersion string
		matches := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindStringSubmatch(con.Data.CompilerVersion)
		if len(matches) != 1 {
			err = fmt.Errorf("can't parse solc version %q", con.Data.CompilerVersion)
			return
		} else {
			targetVersion = matches[0]
		}
		//根据URL获取出源码，为了方便之后测试，把他保存成content，存回一个新的json吧
		codes := con.Data.SourceCodes
		var sourceFiles []contract.SourceFile
		for _, code := range codes {
			rawdata, err := fetchUrl(code.Code)
			if err != nil {
				return err
			}
			file := contract.SourceFile{
				Name:    code.FileName,
				RawCode: rawdata,
			}
			sourceFiles = append(sourceFiles, file)
		}
		addrStarboardContractMap[addr] = &StarboardContract{
			Addr:            addr,
			ContractName:    con.Data.ContractName,
			MetaData:        &metaData,
			CompilerVersion: targetVersion,
			SourceFiles:     sourceFiles,
		}
		time.Sleep(40 * time.Millisecond)
	}
	//这里将map给存起来
	marshal, err := json.Marshal(addrStarboardContractMap)
	if err != nil {
		return err
	}
	err = saveJson(string(marshal))
	return err
}

func handleCSV() (err error) {
	starboardCSV := "path"
	//接下来写读取csv，并建立standardInput
	records, err := readCSV(starboardCSV)
	if err != nil {
		fmt.Println(err)
		return err
	}
	var csvContent []*CsvContent
	csvContent = append(csvContent, &CsvContent{
		address: "padding data",
		content: "padding data",
	})
	skipcount := 0
	for _, record := range records {
		if skipcount > 0 {
			skipcount--
			continue
		}
		if record[1] == "" || record[2] == "" {
			continue
		}
		//如果为Contract ABI则跳过4行
		if record[2] == "Contract ABI" {
			skipcount = 3
			continue
		}
		csvContent = append(csvContent, &CsvContent{
			address: record[1],
			content: record[2],
		})
	}
	//var addrPre, content, addrPtr string = "", "", ""

	length := len(csvContent)
	for i := 1; i < length; i = i + 2 {
		//如果与上一个的地址不一样的话，创建一个[]sourcefile，并与Metadata对应
		//csvContent[i].address !=

	}
	//根据地址从数据库中获取Bytecode把
	println("haha")
	return
}

/*
测试过程用于测试组的变量
*/
var StandInputJsons []contract.StandardInput
var StandInputJsonFiles []*contract.SourceFile
var addrStarboardContractMap map[string]*StarboardContract
var initcodes []string
var contractNames []string
var solcVersion []string
var addrInitcode1237 map[string]string
var v *browser.VerifyContractBiz

const solcSelectPath string = "/Users/jinshi/Library/Python/3.11/bin/solc-select"
const filfoxdir string = "/Users/jinshi/Documents/test/filfox_verify/"
const starboarddir string = "/Users/jinshi/Documents/test/starboard_verify/"

func TestDoSomething(t *testing.T) {
	//initStarboardRes()
	type A struct {
		Name    string `json:"Name,omitempty"`
		Rawcode string `json:"RawCode,omitempty"`
	}
	var a []A
	dir := "/Users/jinshi/Documents/test/filfox_verify/wrong/input.json"
	bytes, err := readFileToBytes(dir)
	if err != nil {
		return
	}
	err = json.Unmarshal(bytes, &a)
	if err != nil {
		return
	}
	for _, file := range a {
		dir := filepath.Dir(file.Name) //这就是获取在服务器中存放合约的位置（自定义）
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return
		}
		var f *os.File
		f, err = os.Create(file.Name) //保存文件到哪儿 这儿还有目录
		if err != nil {
			return
		}
		_, err = io.WriteString(f, file.Rawcode)
		if err != nil {
			return
		}
	}

}

func TestMain(m *testing.M) {
	fmt.Println("write setup code here...") // 测试之前的做一些设置
	// 测试filfox的数据时候，使用TestCompiler_CompileStrandJson测试方法
	err := initFilfoxRes()
	if err != nil {
		return
	}
	//测试starboard的TestCompiler_CompileWithMateData
	//initStarboardRes()
	//用于某些场景用到agg获取数据的场景
	//err := initalAgg()
	//if err != nil {
	//	return
	//}
	retCode := m.Run()                         // 执行测试
	fmt.Println("write teardown code here...") // 测试之后做一些拆卸工作
	os.Exit(retCode)                           // 退出测试
}

func TestJson(t *testing.T) {
	filfoxVerifiedContracts := "/Users/jinshi/Documents/test/filfox_verified_contracts_detail.json"
	var contracts []FilfoxContract
	bytes, err := readFileToBytes(filfoxVerifiedContracts)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = json.Unmarshal(bytes, &contracts)
	if err != nil {
		return
	}

	outputSelection, err := contract.GetDefaultOutputSelection()
	if err != nil {
		return
	}
	for i, con := range contracts {
		input := contract.StandardInput{
			Language: "Solidity",
			Sources:  make(map[string]contract.Source),
			Settings: &contract.Setting{},
		}
		for _, file := range con.SourceFiles {
			rawcode := file.Content
			input.Sources[file.Name] = contract.Source{Content: &rawcode}
		}
		var optDetails contract.OptimizerDetails
		if con.OptDetails != "" {
			err := json.Unmarshal([]byte(con.OptDetails), &optDetails)
			if err != nil {
				return
			}
		}
		input.Settings = &contract.Setting{
			CompilationTarget: nil,
			EvmVersion:        con.EvmVersion,
			Optimizer: &contract.Optimize{
				Enabled: con.Optimize,
				Runs:    con.OptimizeRuns,
				Details: &optDetails,
			},
			OutputSelection: outputSelection,
			ViaIR:           con.ViaIR,
			Metadata: &contract.MetadataSetting{
				UseLiteralContent: true,
			},
		}
		StandInputJsons = append(StandInputJsons, input)
		var rawcode []byte
		rawcode, err = json.Marshal(input)
		if err != nil {
			return
		}
		sfile := contract.SourceFile{
			Name:    "standardJson_" + strconv.Itoa(i) + ".json",
			RawCode: string(rawcode),
		}
		StandInputJsonFiles = append(StandInputJsonFiles, &sfile)
		initcodes = append(initcodes, con.InitCode)
		contractNames = append(contractNames, con.ContractName)
	}
	//获得了所有的对象，接下来将它变成SourceFile的数组

	if len(contracts) > 2 {
		name := contracts[0].ContractName
		fmt.Println(name)
	}
	println("end")

}

func TestCompiler_CompileStrandJson(t *testing.T) {
	c := contract.Compiler{
		Solidity: &contract.Solidity{
			Path: "/Users/jinshi/Library/Python/3.11/bin/solc",
		},
		Optimize: &contract.Optimize{
			Enabled: false,
			Runs:    0,
			Details: &contract.OptimizerDetails{},
		},
		Config:     nil,
		IsMetaData: false,
	}
	tt := struct {
		count       int
		name        []string
		file        []*contract.SourceFile
		initcode    []string
		solcVersion []string
	}{len(contractNames),
		contractNames,
		StandInputJsonFiles,
		initcodes,
		solcVersion,
	}
	err := c.ChangeVersion(solcSelectPath, tt.solcVersion[39])
	if err != nil {
		return
	}
	gotCompiledFiles, _ := c.CompileStandardJson(tt.file[39], filfoxdir)
	var isVerified = false
	initCodeNoCBOR := contract.RemoveCBORCode(tt.initcode[39])
	for _, compiledContract := range gotCompiledFiles {
		byteCodeNoCBOR := contract.RemoveCBORCode(compiledContract.ByteCode)
		if len(initCodeNoCBOR) >= len(byteCodeNoCBOR) && byteCodeNoCBOR != compiledContract.ByteCode {
			compareCode := initCodeNoCBOR[:len(byteCodeNoCBOR)]
			compareCode, byteCodeNoCBOR = contract.RemovePlaceholderCode(compareCode, byteCodeNoCBOR)
			if compareCode == byteCodeNoCBOR {
				isVerified = true
				break
			}
		}
	}
	println(isVerified)

	for i := 0; i < tt.count; i++ {
		t.Run(tt.name[i], func(t *testing.T) {
			fmt.Println(tt.solcVersion[i])
			err := c.ChangeVersion(solcSelectPath, tt.solcVersion[i])
			if err != nil {
				t.Errorf("Compiler.CompileStandardJson() error = %v", err)
				return
			}
			gotCompiledFiles, err := c.CompileStandardJson(tt.file[i], filfoxdir)
			if err != nil {
				t.Errorf("Compiler.CompileStandardJson() error = %v", err)
				return
			}
			var isVerified = false
			initCodeNoCBOR := contract.RemoveCBORCode(tt.initcode[i])
			for _, compiledContract := range gotCompiledFiles {
				byteCodeNoCBOR := contract.RemoveCBORCode(compiledContract.ByteCode)
				if len(initCodeNoCBOR) >= len(byteCodeNoCBOR) && byteCodeNoCBOR != compiledContract.ByteCode {
					compareCode := initCodeNoCBOR[:len(byteCodeNoCBOR)]
					compareCode, byteCodeNoCBOR = contract.RemovePlaceholderCode(compareCode, byteCodeNoCBOR)
					//assert.Equal(t, compareCode, byteCodeNoCBOR)
					if compareCode == byteCodeNoCBOR {
						isVerified = true
						break
					}
				}
			}
			assert.Equal(t, true, isVerified)
		})
	}
}

func TestCompiler_CompileWithMateData(t *testing.T) {
	c := contract.Compiler{
		Solidity: &contract.Solidity{
			Path: "/Users/jinshi/Library/Python/3.11/bin/solc",
		},
		Optimize: &contract.Optimize{
			Enabled: false,
			Runs:    0,
			Details: &contract.OptimizerDetails{},
		},
		Config:     nil,
		IsMetaData: false,
	}
	/*
		此处用于单个测试用例的调试，某些情况，子测试并不能很好的调试(可能是编译器的问题)
	*/
	//tcode := "0x60806040526040516107353803806107358339810160408190526100229161031e565b61002e82826000610035565b505061043b565b61003e8361006b565b60008251118061004b5750805b156100665761006483836100ab60201b6100291760201c565b505b505050565b610074816100d7565b6040516001600160a01b038216907fbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b90600090a250565b60606100d0838360405180606001604052806027815260200161070e602791396101a9565b9392505050565b6100ea8161022260201b6100551760201c565b6101515760405162461bcd60e51b815260206004820152602d60248201527f455243313936373a206e657720696d706c656d656e746174696f6e206973206e60448201526c1bdd08184818dbdb9d1c9858dd609a1b60648201526084015b60405180910390fd5b806101887f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc60001b61023160201b6100641760201c565b80546001600160a01b0319166001600160a01b039290921691909117905550565b6060600080856001600160a01b0316856040516101c691906103ec565b600060405180830381855af49150503d8060008114610201576040519150601f19603f3d011682016040523d82523d6000602084013e610206565b606091505b50909250905061021886838387610234565b9695505050505050565b6001600160a01b03163b151590565b90565b606083156102a0578251610299576001600160a01b0385163b6102995760405162461bcd60e51b815260206004820152601d60248201527f416464726573733a2063616c6c20746f206e6f6e2d636f6e74726163740000006044820152606401610148565b50816102aa565b6102aa83836102b2565b949350505050565b8151156102c25781518083602001fd5b8060405162461bcd60e51b81526004016101489190610408565b634e487b7160e01b600052604160045260246000fd5b60005b8381101561030d5781810151838201526020016102f5565b838111156100645750506000910152565b6000806040838503121561033157600080fd5b82516001600160a01b038116811461034857600080fd5b60208401519092506001600160401b038082111561036557600080fd5b818501915085601f83011261037957600080fd5b81518181111561038b5761038b6102dc565b604051601f8201601f19908116603f011681019083821181831017156103b3576103b36102dc565b816040528281528860208487010111156103cc57600080fd5b6103dd8360208301602088016102f2565b80955050505050509250929050565b600082516103fe8184602087016102f2565b9190910192915050565b60208152600082518060208401526104278160408501602087016102f2565b601f01601f19169190910160400192915050565b6102c48061044a6000396000f3fe60806040523661001357610011610017565b005b6100115b610027610022610067565b61009f565b565b606061004e8383604051806060016040528060278152602001610268602791396100c3565b9392505050565b6001600160a01b03163b151590565b90565b600061009a7f360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc546001600160a01b031690565b905090565b3660008037600080366000845af43d6000803e8080156100be573d6000f35b3d6000fd5b6060600080856001600160a01b0316856040516100e09190610218565b600060405180830381855af49150503d806000811461011b576040519150601f19603f3d011682016040523d82523d6000602084013e610120565b606091505b50915091506101318683838761013b565b9695505050505050565b606083156101ac5782516101a5576001600160a01b0385163b6101a55760405162461bcd60e51b815260206004820152601d60248201527f416464726573733a2063616c6c20746f206e6f6e2d636f6e747261637400000060448201526064015b60405180910390fd5b50816101b6565b6101b683836101be565b949350505050565b8151156101ce5781518083602001fd5b8060405162461bcd60e51b815260040161019c9190610234565b60005b838110156102035781810151838201526020016101eb565b83811115610212576000848401525b50505050565b6000825161022a8184602087016101e8565b9190910192915050565b60208152600082518060208401526102538160408501602087016101e8565b601f01601f1916919091016040019291505056fe416464726573733a206c6f772d6c6576656c2064656c65676174652063616c6c206661696c6564a26469706673582212208363a6b4e26aedf338e012e3e06817685bd754dbbaf9e2e1fb491b048115c98e64736f6c63430008090033416464726573733a206c6f772d6c6576656c2064656c65676174652063616c6c206661696c6564"
	//a := "0x34150fded1e598866e5111718e4f5d5af2517f98"
	//tt := addrStarboardContractMap[a]
	//var sourceFiles []contract.SourceFile
	//for _, tmpFile := range tt.SourceFiles {
	//	sourceFiles = append(sourceFiles, contract.SourceFile{
	//		Name:    tmpFile.Name,
	//		RawCode: tmpFile.RawCode,
	//	})
	//}
	//err := c.ChangeVersion(solcSelectPath, tt.CompilerVersion)
	//if err != nil {
	//	return
	//}
	//gotCompiledFiles, err := c.CompileWithMetaData(sourceFiles, *tt.MetaData, starboarddir)
	//if err != nil {
	//	t.Errorf("Compiler.CompileWithMetaData() error = %v", err)
	//	return
	//}
	//initCodeNoCBOR := contract.RemoveCBORCode(tcode)
	//compiledContract := gotCompiledFiles[0]
	//byteCodeNoCBOR := contract.RemoveCBORCode(compiledContract.ByteCode)
	//if len(initCodeNoCBOR) >= len(byteCodeNoCBOR) && byteCodeNoCBOR != compiledContract.ByteCode {
	//	compareCode := initCodeNoCBOR[:len(byteCodeNoCBOR)]
	//	compareCode, byteCodeNoCBOR = contract.RemovePlaceholderCode(compareCode, byteCodeNoCBOR)
	//	assert.Equal(t, compareCode, byteCodeNoCBOR)
	//}

	for addr, initcode := range addrInitcode1237 {
		tt := addrStarboardContractMap[addr]
		var sourceFiles []contract.SourceFile
		for _, tmpFile := range tt.SourceFiles {
			sourceFiles = append(sourceFiles, contract.SourceFile{
				Name:    tmpFile.Name,
				RawCode: tmpFile.RawCode,
			})
		}

		t.Run(tt.ContractName+"_"+tt.Addr, func(t *testing.T) {
			isVerified := false
			err := c.ChangeVersion(solcSelectPath, tt.CompilerVersion)
			if err != nil {
				t.Errorf("Compiler.CompileWithMetaData() error = %v", err)
				return
			}
			gotCompiledFiles, err := c.CompileWithMetaData(sourceFiles, *tt.MetaData, starboarddir)
			if err != nil {
				t.Errorf("Compiler.CompileWithMetaData() error = %v", err)
				return
			}

			initCodeNoCBOR := contract.RemoveCBORCode(initcode)
			compiledContract := gotCompiledFiles[0]
			byteCodeNoCBOR := contract.RemoveCBORCode(compiledContract.ByteCode)
			if len(initCodeNoCBOR) >= len(byteCodeNoCBOR) && byteCodeNoCBOR != compiledContract.ByteCode {
				compareCode := initCodeNoCBOR[:len(byteCodeNoCBOR)]
				fmt.Println("进行了initcode匹配")
				compareCode, byteCodeNoCBOR = contract.RemovePlaceholderCode(compareCode, byteCodeNoCBOR)
				assert.Equal(t, compareCode, byteCodeNoCBOR)
				isVerified = true
			}
			if !reflect.DeepEqual(true, isVerified) {
				t.Errorf("Compiler.CompileStrandJsonWithTarget %v ERROR!", tt.ContractName)
			}
			assert.Equal(t, true, isVerified)
			fmt.Println("验证成功")
		})

		//for addr, tt := range addrStarboardContractMap {
		//	var sourceFiles []contract.SourceFile
		//	for _, tmpFile := range tt.SourceFiles {
		//		sourceFiles = append(sourceFiles, contract.SourceFile{
		//			Name:    tmpFile.Name,
		//			RawCode: tmpFile.RawCode,
		//		})
		//	}
		//	t.Run(tt.ContractName, func(t *testing.T) {
		//		err := c.ChangeVersion(solcSelectPath, tt.CompilerVersion)
		//		if err != nil {
		//			return
		//		}
		//		gotCompiledFiles, err := c.CompileWithMetaData(sourceFiles, *tt.MetaData, starboarddir)
		//		if err != nil {
		//			t.Errorf("Compiler.CompileWithMetaData() error = %v", err)
		//			return
		//		}
		//		actor, err := v.GetOnChainActor(context.Background(), addr)
		//		if err != nil {
		//			return
		//		}
		//		initCodeNoCBOR := "0x" + actor.InitCode.InitCode
		//		compiledContract := gotCompiledFiles[0]
		//		byteCodeNoCBOR := contract.RemoveCBORCode(compiledContract.ByteCode)
		//		if len(initCodeNoCBOR) >= len(byteCodeNoCBOR) && byteCodeNoCBOR != compiledContract.ByteCode {
		//			compareCode := initCodeNoCBOR[:len(byteCodeNoCBOR)]
		//			compareCode, byteCodeNoCBOR = contract.RemovePlaceholderCode(compareCode, byteCodeNoCBOR)
		//			assert.Equal(t, compareCode, byteCodeNoCBOR)
		//		}
		//	})
		//}
	}
	fmt.Println(addrInitcode1237)
}
