package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-version"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/contract"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func NewVerifyContract(agg londobell.Agg, adapter londobell.Adapter, db *gorm.DB, conf *config.Config) *VerifyContractBiz {
	return &VerifyContractBiz{
		agg:         agg,
		adapter:     adapter,
		contractsDB: dal.NewEvmContractDal(db),
		config:      conf,
		db:          db,
	}
}

var _ filscan.VerifyContractAPI = (*VerifyContractBiz)(nil)

type VerifyContractBiz struct {
	agg         londobell.Agg
	adapter     londobell.Adapter
	contractsDB repository.EvmContractRepo
	config      *config.Config
	db          *gorm.DB
}

func (v VerifyContractBiz) VerifiedContractByActorID(ctx context.Context, request filscan.VerifiedContractRequest) (resp filscan.VerifiedContractResponse, err error) {
	evmContract, err := v.contractsDB.SelectFEvmContractsByActorID(ctx, request.InputAddress)
	if err != nil {
		return
	}
	sols, err := v.contractsDB.SelectFEvmContractSolsByActorID(ctx, request.InputAddress)
	if err != nil {
		return
	}
	var verifiedContract *filscan.VerifiedContractResponse
	if evmContract != nil && sols != nil {
		convertor := assembler.VerifyContact{}
		verifiedContract = convertor.VerifiedContractsToCompiledFile(evmContract, sols)
	}
	if verifiedContract != nil {
		resp = *verifiedContract
	} else {
		var byteCode *londobell.InitCode
		byteCode, err = v.agg.InitCodeForEvm(ctx, chain.SmartAddress(request.InputAddress))
		if err != nil {
			return
		}
		if byteCode != nil {
			unVerifiedContract := filscan.VerifiedContractResponse{
				CompiledFile: &filscan.CompiledFile{
					ByteCode: byteCode.InitCode,
				},
			}
			resp = unVerifiedContract
		}
	}

	return
}
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

// 获取给测试用的地址及Initcode
func (v VerifyContractBiz) getAllInitCode() {
	type StarboardContract struct {
		Addr            string                `json:"addr,omitempty"`
		ContractName    string                `json:"contractName,omitempty"`
		MetaData        *contract.MetaData    `json:"metaData,omitempty"`
		CompilerVersion string                `json:"compilerVersion,omitempty"`
		SourceFiles     []contract.SourceFile `json:"sourceFiles,omitempty"`
	}
	var addrToCode = make(map[string]string)
	starboardVerifiedContracts := "/Users/jinshi/Downloads/filscan-backend/modules/filscan/domain/contract/test/1691657825starboardContract.json"
	addrStarboardContractMap := make(map[string]*StarboardContract)
	bytes, err := readFileToBytes(starboardVerifiedContracts)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = json.Unmarshal(bytes, &addrStarboardContractMap)
	if err != nil {
		return
	}
	var actor *contract.OnChainActor
	for addr := range addrStarboardContractMap {
		actor, err = v.GetOnChainActor(context.Background(), addr)
		if err != nil {
			return
		}
		if actor != nil {
			addrToCode[addr] = "0x" + actor.InitCode.InitCode
		}
	}
	marshal, err := json.Marshal(addrToCode)
	if err != nil {
		return
	}
	filename := "/Users/jinshi/Downloads/filscan-backend/modules/filscan/domain/contract/test/addr_code1238.json"
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
	_, err = io.WriteString(file, string(marshal))
	if err != nil {
		return
	}
	return
	//initCodeNoCBOR := "0x" + actor.InitCode.InitCode
}

func (v VerifyContractBiz) VerifiedContractList(ctx context.Context, request filscan.VerifiedContractListRequest) (resp filscan.VerifiedContractListResponse, err error) {
	contracts, total, err := v.contractsDB.SelectVerifiedFEvmContracts(ctx, request.Index, request.Limit)
	if err != nil {
		return
	}
	var compiledFiles []*filscan.CompiledFile
	if contracts != nil {
		compiledFiles = assembler.VerifyContact{}.VerifiedContractsToCompiledFileList(contracts)
	}
	resp.CompiledFileList = compiledFiles
	resp.Total = total

	return
}

func (v VerifyContractBiz) SolidityVersions(ctx context.Context, request filscan.VersionListRequest) (resp filscan.VersionListResponse, err error) {
	var versionList contract.SolidityVersions
	list := versionList.GetVersionList()
	if err != nil {
		return
	}
	resp.VersionList = list
	return
}

func (v VerifyContractBiz) Licenses(ctx context.Context, request filscan.VersionListRequest) (resp filscan.VersionListResponse, err error) {
	var LicenseList contract.Licenses
	list := LicenseList.GetLicenseList()
	if err != nil {
		return
	}
	resp.VersionList = list
	return
}

// VerifyHardhatContract
/*
	可传入build-info文件，解析出standar-json标准输入，然后执行，比较。此处用不到Metadata
	此处也用不到传入文件，只需要传入build-info即可
	保留文件是为了之后通过Metadata与文件进行编译
*/
func (v VerifyContractBiz) VerifyHardhatContract(ctx context.Context, request filscan.VerifyHardhatContractRequest) (resp filscan.VerifyContractResponse, err error) {
	//v.getAllInitCode()
	target := ""
	input := strings.TrimSpace(request.ContractAddress)
	checkIsVerified, err := v.checkIsVerified(ctx, input)
	if err != nil {
		return
	}
	if checkIsVerified != nil {
		resp.IsVerified = true
		resp.CompiledFile = checkIsVerified
		return
	}
	var actor *contract.OnChainActor
	actor, err = v.GetOnChainActor(ctx, input)
	if err != nil {
		return
	}

	var actorID, actorAddress, ethAddress, initCode string
	if actor != nil {
		actorID = actor.ActorID
		actorAddress = actor.ActorAddress
		ethAddress = actor.EthAddress
		if actor.InitCode != nil {
			initCode = "0x" + actor.InitCode.InitCode
		}
	} else {
		resp.IsVerified = false
		err = fmt.Errorf("can't find the input address %s, maybe it have not created a contract yet", input)
		return
	}
	fileDir := *v.config.Solidity.ContractDirectory + strconv.FormatUint(uint64(time.Now().Unix()), 10) + "/"
	var stdFile *contract.SourceFile
	var hardhatInput contract.HardhatInput
	if request.HardhatBuildInfoFile != nil {
		err = json.Unmarshal([]byte(request.HardhatBuildInfoFile.SourceCode), &hardhatInput)
		if err != nil {
			return
		}
		marshal, err1 := json.Marshal(hardhatInput.Input)
		if err1 != nil {
			return resp, err1
		}
		stdFile = &contract.SourceFile{
			Name:    fileDir + request.HardhatBuildInfoFile.FileName,
			RawCode: string(marshal),
		}
	} else {
		resp.IsVerified = false
		err = fmt.Errorf("please input a hardhat build-info file")
		return
	}
	var metaDataFile *contract.SourceFile
	var isMetaData bool
	if request.MateDataFile != nil {
		metaDataFile = &contract.SourceFile{
			Name:    fileDir + request.MateDataFile.FileName,
			RawCode: request.MateDataFile.SourceCode,
		}
		isMetaData = true
	}
	solcPath := v.config.Solidity.SolcPath
	var solidity *contract.Solidity
	solidity, err = contract.NewSolidity(*solcPath)
	if err != nil {
		return
	}
	compiler := &contract.Compiler{
		Solidity: solidity,
		Optimize: &contract.Optimize{
			Enabled: request.Optimize,
			Runs:    request.OptimizeRuns,
		},
		Config:     v.config,
		IsMetaData: isMetaData,
	}

	var targetVersion string
	//直接使用build-info中的版本，不需要使用request中传入的各种参数
	//matches := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindStringSubmatch(request.CompileVersion)
	//if len(matches) != 1 {
	//	err = fmt.Errorf("can't parse solc version %q", request.CompileVersion)
	//	return
	//} else {
	//	targetVersion = matches[0]
	//}
	targetVersion = hardhatInput.SolcVersion
	solcSelectPath := *v.config.Solidity.SolcSelectPath
	err = compiler.ChangeVersion(solcSelectPath, targetVersion)
	if err != nil {
		return
	}
	var compiledContracts []*contract.CompiledFile
	compiledContracts, err = v.compileHardhatBuildInfo(compiler, targetVersion, stdFile, metaDataFile, fileDir)
	if err != nil {
		resp.IsVerified = false
		return
	}

	var comparedContracts []*contract.CompiledFile
	var returnContracts []*contract.CompiledFile
	var mainContract *contract.CompiledFile
	if compiledContracts != nil {
		const oldVersion = "0.5.11"
		var comparedVersion *version.Version
		comparedVersion, err = version.NewVersion(oldVersion)
		if err != nil {
			return
		}
		var inputVersion *version.Version
		inputVersion, err = version.NewVersion(targetVersion)
		if err != nil {
			return
		}
		if inputVersion.GreaterThan(comparedVersion) {
			comparedContracts = contract.CompareByteCodeWithTarget(compiledContracts, initCode, target) //这里去判断是否主合约，即字节码相同的。因为会去比较很多个文件
		} else {
			comparedContracts = contract.CompareByteCodeOldVersionWithTarget(compiledContracts, initCode, target)
		}
		for _, comparedContract := range comparedContracts {
			if comparedContract.IsMainContract {
				mainContract = comparedContract
				target = mainContract.ContractName
				break
			}
		}
	} else {
		resp.IsVerified = false
		err = fmt.Errorf("the input files's bytecode is different from the onchain byte code")
		return
	}

	if mainContract != nil {
		//err = v.saveMainContract(ctx, request, mainContract, actor)
		//通过Metadata中的地址和 compilefile中的solidityFileName去对比
		var tmpMetadata contract.MetaData
		for _, contractMap := range hardhatInput.Ouput.Contracts {
			cont, ok := contractMap[target]
			if ok {
				err = json.Unmarshal([]byte(cont.MetaDataString), &tmpMetadata)
				if err != nil {
					return
				}
				break
			}
		}
		for _, compiledContract := range compiledContracts {
			if _, ok := tmpMetadata.Sources[compiledContract.ContractFile.Name]; ok {
				returnContracts = append(returnContracts, compiledContract)
			}
		}
		fEVMContract := &po.FEvmContracts{
			ActorID:         actorID,
			ActorAddress:    actorAddress,
			ContractAddress: ethAddress,
			Arguments:       mainContract.Arguments,
			License:         request.License,
			Language:        "Solidity",
			Compiler:        targetVersion,
			Optimize:        compiler.Optimize.Enabled,
			OptimizeRuns:    compiler.Optimize.Runs,
			Verified:        true,
			CreatedAt:       time.Now(),
		}

		var fEVMContractSols []*po.FEvmContractSols
		for _, returnContract := range returnContracts {
			fEVMContractSols = append(fEVMContractSols, &po.FEvmContractSols{
				ActorID:        actorID,
				FileName:       returnContract.ContractFile.Name,
				Source:         returnContract.ContractFile.RawCode,
				Size:           int64(types.SizeOfStdString(returnContract.ContractFile.RawCode)),
				ContractName:   returnContract.ContractName,
				ByteCode:       returnContract.ByteCode,
				ABI:            returnContract.ABI,
				CreatedAt:      time.Now(),
				IsMainContract: returnContract.IsMainContract,
			})
		}

		tx := v.db.Begin()
		ctx = _dal.ContextWithDB(context.Background(), tx)
		defer func() {
			if err != nil {
				tx.Rollback()
			} else {
				tx.Commit()
			}
		}()

		err = v.contractsDB.SaveFEvmContracts(ctx, fEVMContract)
		if err != nil {
			resp.IsVerified = false
			return
		}
		err = v.contractsDB.SaveFEvmContractSols(ctx, fEVMContractSols)
		if err != nil {
			resp.IsVerified = false
			return
		}

		resp = filscan.VerifyContractResponse{
			IsVerified: true,
			CompiledFile: &filscan.CompiledFile{
				ActorID:         actorID,
				ActorAddress:    actorAddress,
				ContractAddress: ethAddress,
				Arguments:       mainContract.Arguments,
				License:         request.License,
				Language:        "Solidity",
				Compiler:        request.CompileVersion,
				Optimize:        compiler.Optimize.Enabled,
				OptimizeRuns:    compiler.Optimize.Runs,
				ContractName:    mainContract.ContractName,
				ByteCode:        initCode,
				ABI:             mainContract.ABI,
				LocalByteCode:   initCode,
			},
		}
	} else {
		resp.IsVerified = false
		resp.CompiledFile = &filscan.CompiledFile{
			ActorID:         actorID,
			ActorAddress:    actorAddress,
			ContractAddress: ethAddress,
			ByteCode:        initCode,
		}
		err = fmt.Errorf("local compiled result is different from the on chain data")
		return
	}
	return
}

//func (v VerifyContractBiz) saveMainContract(ctx context.Context, request filscan.VerifyHardhatContractRequest, mainContract *contract.CompiledFile, actor *contract.OnChainActor) (err error) {
//	fEVMContract := &po.FEvmContracts{
//		ActorID:         actor.ActorID,
//		ActorAddress:    actor.ActorAddress,
//		ContractAddress: actor.EthAddress,
//		Arguments:       mainContract.Arguments,
//		License:         request.License,
//		Language:        "Solidity",
//		Compiler:        request.CompileVersion,
//		Optimize:        request.Optimize,
//		OptimizeRuns:    request.OptimizeRuns,
//		Verified:        true,
//		CreatedAt:       time.Now(),
//	}
//
//	var fEVMContractSols []*po.FEvmContractSols
//	fEVMContractSols = append(fEVMContractSols, &po.FEvmContractSols{
//		ActorID:        actor.ActorID,
//		FileName:       mainContract.ContractFile.Name,
//		Source:         mainContract.ContractFile.RawCode,
//		Size:           int64(types.SizeOfStdString(mainContract.ContractFile.RawCode)),
//		ContractName:   mainContract.ContractName,
//		ByteCode:       mainContract.ByteCode,
//		ABI:            mainContract.ABI,
//		CreatedAt:      time.Now(),
//		IsMainContract: mainContract.IsMainContract,
//	})
//
//	tx := v.db.Begin()
//	ctx = _dal.ContextWithDB(context.Background(), tx)
//	defer func() {
//		if err != nil {
//			tx.Rollback()
//		} else {
//			tx.Commit()
//		}
//	}()
//
//	err = v.contractsDB.SaveFEvmContracts(ctx, fEVMContract)
//	if err != nil {
//		return
//	}
//	return v.contractsDB.SaveFEvmContractSols(ctx, fEVMContractSols)
//
//}

func (v VerifyContractBiz) VerifyContract(ctx context.Context, request filscan.VerifyContractRequest) (resp filscan.VerifyContractResponse, err error) {
	target := "" //target可以直接从Metadata中获取，也可以从编译后的主合约中获取（用于快速定位是编译哪个合约及找到hardhat是哪个Metadata）
	input := strings.TrimSpace(request.ContractAddress)
	checkIsVerified, err := v.checkIsVerified(ctx, input)
	if err != nil {
		return
	}
	if checkIsVerified != nil {
		resp.IsVerified = true
		resp.CompiledFile = checkIsVerified
		return
	}
	var actor *contract.OnChainActor
	actor, err = v.GetOnChainActor(ctx, input)
	if err != nil {
		return
	}
	var actorID, actorAddress, ethAddress, initCode string
	if actor != nil {
		actorID = actor.ActorID
		actorAddress = actor.ActorAddress
		ethAddress = actor.EthAddress
		if actor.InitCode != nil {
			initCode = "0x" + actor.InitCode.InitCode
		}
	} else {
		resp.IsVerified = false
		err = fmt.Errorf("can't find the input address %s, maybe it have not created a contract yet", input)
		return
	}

	fileDir := *v.config.Solidity.ContractDirectory + strconv.FormatUint(uint64(time.Now().Unix()), 10) + "/"
	var sourceFile []contract.SourceFile
	if request.SourceFile != nil {
		for _, file := range request.SourceFile {
			sourceFile = append(sourceFile, contract.SourceFile{
				Name:    fileDir + file.FileName,
				RawCode: file.SourceCode,
			})
		}
	} else {
		resp.IsVerified = false
		err = fmt.Errorf("please input a solidity file")
		return
	}
	var metaDataFile *contract.SourceFile
	var isMetaData bool
	if request.MateDataFile != nil {
		metaDataFile = &contract.SourceFile{
			Name:    fileDir + request.MateDataFile.FileName,
			RawCode: request.MateDataFile.SourceCode,
		}
		isMetaData = true
		var tmpMetadata contract.MetaData
		err = json.Unmarshal([]byte(request.MateDataFile.SourceCode), &tmpMetadata)
		if err != nil || tmpMetadata.Sources == nil {
			err = fmt.Errorf("metadata file parse error")
			return
		}
		for _, v := range tmpMetadata.Settings.CompilationTarget {
			target = v
		}
	}

	solcPath := v.config.Solidity.SolcPath
	var solidity *contract.Solidity
	solidity, err = contract.NewSolidity(*solcPath)
	if err != nil {
		return
	}

	compiler := &contract.Compiler{
		Solidity: solidity,
		Optimize: &contract.Optimize{
			Enabled: request.Optimize,
			Runs:    request.OptimizeRuns,
		},
		Config:     v.config,
		IsMetaData: isMetaData,
	}

	var targetVersion string
	matches := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindStringSubmatch(request.CompileVersion)
	if len(matches) != 1 {
		err = fmt.Errorf("can't parse solc version %q", request.CompileVersion)
		return
	} else {
		targetVersion = matches[0]
	}

	var compiledContracts []*contract.CompiledFile
	compiledContracts, err = v.compileContract(compiler, targetVersion, sourceFile, metaDataFile, fileDir)
	if err != nil {
		resp.IsVerified = false
		return
	}

	var comparedContracts []*contract.CompiledFile
	var mainContract *contract.CompiledFile
	if compiledContracts != nil {
		const oldVersion = "0.5.11"
		var comparedVersion *version.Version
		comparedVersion, err = version.NewVersion(oldVersion)
		if err != nil {
			return
		}
		var inputVersion *version.Version
		inputVersion, err = version.NewVersion(targetVersion)
		if err != nil {
			return
		}
		//todo 如果有目标target，就直接根据target去找，否则遍历。有target的时候，comparedContracts只返回一个
		if inputVersion.GreaterThan(comparedVersion) {
			comparedContracts = contract.CompareByteCodeWithTarget(compiledContracts, initCode, target)
		} else {
			comparedContracts = contract.CompareByteCodeOldVersionWithTarget(compiledContracts, initCode, target)
		}
		//引入了target之后，有Metadata情况下，这里一般只有一个元素
		for _, comparedContract := range comparedContracts {
			if comparedContract.IsMainContract {
				mainContract = comparedContract
				break
			}
		}
	} else {
		resp.IsVerified = false
		err = fmt.Errorf("the input files's bytecode is different from the onchain byte code")
		return
	}

	if mainContract != nil {
		fEVMContract := &po.FEvmContracts{
			ActorID:         actorID,
			ActorAddress:    actorAddress,
			ContractAddress: ethAddress,
			Arguments:       mainContract.Arguments,
			License:         request.License,
			Language:        "Solidity",
			Compiler:        request.CompileVersion,
			Optimize:        compiler.Optimize.Enabled,
			OptimizeRuns:    compiler.Optimize.Runs,
			Verified:        true,
			CreatedAt:       time.Now(),
		}

		var fEVMContractSols []*po.FEvmContractSols
		for _, compiledContract := range compiledContracts {
			fEVMContractSols = append(fEVMContractSols, &po.FEvmContractSols{
				ActorID:        actorID,
				FileName:       compiledContract.ContractFile.Name,
				Source:         compiledContract.ContractFile.RawCode,
				Size:           int64(types.SizeOfStdString(compiledContract.ContractFile.RawCode)),
				ContractName:   compiledContract.ContractName,
				ByteCode:       compiledContract.ByteCode,
				ABI:            compiledContract.ABI,
				CreatedAt:      time.Now(),
				IsMainContract: compiledContract.IsMainContract,
			})
		}

		tx := v.db.Begin()
		ctx = _dal.ContextWithDB(context.Background(), tx)
		defer func() {
			if err != nil {
				tx.Rollback()
			} else {
				tx.Commit()
			}
		}()

		err = v.contractsDB.SaveFEvmContracts(ctx, fEVMContract)
		if err != nil {
			resp.IsVerified = false
			return
		}
		err = v.contractsDB.SaveFEvmContractSols(ctx, fEVMContractSols)
		if err != nil {
			resp.IsVerified = false
			return
		}

		resp = filscan.VerifyContractResponse{
			IsVerified: true,
			CompiledFile: &filscan.CompiledFile{
				ActorID:         actorID,
				ActorAddress:    actorAddress,
				ContractAddress: ethAddress,
				Arguments:       mainContract.Arguments,
				License:         request.License,
				Language:        "Solidity",
				Compiler:        request.CompileVersion,
				Optimize:        compiler.Optimize.Enabled,
				OptimizeRuns:    compiler.Optimize.Runs,
				ContractName:    mainContract.ContractName,
				ByteCode:        initCode,
				ABI:             mainContract.ABI,
				LocalByteCode:   initCode,
			},
		}
	} else {
		resp.IsVerified = false
		resp.CompiledFile = &filscan.CompiledFile{
			ActorID:         actorID,
			ActorAddress:    actorAddress,
			ContractAddress: ethAddress,
			ByteCode:        initCode,
		}
		err = fmt.Errorf("local compiled result is different from the on chain data")
		return
	}

	return
}

func (v VerifyContractBiz) compileHardhatBuildInfo(compiler *contract.Compiler, targetVersion string, stdInputFile *contract.SourceFile, metaDataFile *contract.SourceFile, fileDir string) (compiledFiles []*contract.CompiledFile, err error) {
	solcSelectPath := *v.config.Solidity.SolcSelectPath
	err = compiler.ChangeVersion(solcSelectPath, targetVersion)
	if err != nil {
		return
	}
	compiledFiles, err = compiler.CompileStandardJson(stdInputFile, fileDir)
	return
}

func (v VerifyContractBiz) compileContract(compiler *contract.Compiler, targetVersion string, files []contract.SourceFile, metaDataFile *contract.SourceFile, fileDir string) (compiledFiles []*contract.CompiledFile, err error) {
	solcSelectPath := *v.config.Solidity.SolcSelectPath
	err = compiler.ChangeVersion(solcSelectPath, targetVersion)
	if err != nil {
		return
	}
	if metaDataFile == nil {
		compiledFiles, err = compiler.CompileSourceFile(files, fileDir)
		if err != nil {
			//err = fmt.Errorf("the input source files couldn't be compiled")
			return
		}
	} else {
		var metaData contract.MetaData
		metaData, err = compiler.CreatedMateDataFile(*metaDataFile)
		if err != nil {
			return nil, err
		}
		metaDataVersion := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindStringSubmatch(metaData.Compiler.Version)
		if metaDataVersion[0] != targetVersion {
			err = fmt.Errorf("input compiler version is different from the metadata compiler version")
			return
		}

		if metaData.Settings.Optimizer != nil { //Metadata使用了优化情况
			if metaData.Settings.Optimizer.Enabled != compiler.Optimize.Enabled {
				err = fmt.Errorf("input optimize setting is different from the metadata optimize setting")
				return
			}
			// optimizer.enable可以为false，但run为200。因为Enabled只对run有影响，对detail没影响。所以可以enable为false但是detail有
			//这里只是用来判断参数不匹配情况，编译器只需要参数就好
			if metaData.Settings.Optimizer.Enabled && metaData.Settings.Optimizer.Runs != compiler.Optimize.Runs {
				err = fmt.Errorf("input optimize setting is different from the metadata optimize setting")
				return
			}
		} else {
			//判断是否前端传来的也是优化也是false
			if compiler.Optimize.Enabled {
				err = fmt.Errorf("input optimize setting is different from the metadata optimize setting")
				return
			}
		}
		compiledFiles, err = compiler.CompileWithMetaData(files, metaData, fileDir)
		if err != nil {
			//err = fmt.Errorf("the input source files couldn't be compiled")
			return
		}
	}

	return
}

func (v VerifyContractBiz) GetOnChainActor(ctx context.Context, input string) (actor *contract.OnChainActor, err error) {
	inputActor, err := v.getInputActor(ctx, input)
	if err != nil {
		return
	}
	var initCode *londobell.InitCode
	var ethAddress string
	if inputActor != nil {
		initCode, err = v.agg.InitCodeForEvm(ctx, chain.SmartAddress(inputActor.ActorID))
		if err != nil {
			return
		}
		ethAddress, err = TransferToETHAddress(inputActor.DelegatedAddr)
		if err != nil {
			return
		}
	}
	if initCode != nil && ethAddress != "" {
		actor = &contract.OnChainActor{
			ActorID:      inputActor.ActorID,
			ActorAddress: inputActor.DelegatedAddr,
			EthAddress:   ethAddress,
			InitCode:     initCode,
		}
	}
	return
}

func (v VerifyContractBiz) checkIsVerified(ctx context.Context, input string) (resp *filscan.CompiledFile, err error) {
	inputActor, err := v.getInputActor(ctx, input)
	if err != nil {
		return
	}
	if inputActor != nil {
		var contracts *po.FEvmContracts
		contracts, err = v.contractsDB.SelectFEvmContractsByActorID(ctx, inputActor.ActorID)
		if err != nil {
			return
		}
		var sols []*po.FEvmContractSols
		sols, err = v.contractsDB.SelectFEvmContractSolsByActorID(ctx, inputActor.ActorID)
		if err != nil {
			return
		}
		if contracts != nil && sols != nil {
			var contractName, byteCode, ABI string
			for _, sol := range sols {
				if sol.IsMainContract {
					contractName = sol.ContractName
					byteCode = sol.ByteCode
					ABI = sol.ABI
				}
			}
			resp = &filscan.CompiledFile{
				ActorID:         contracts.ActorID,
				ActorAddress:    contracts.ActorAddress,
				ContractAddress: contracts.ContractAddress,
				Arguments:       contracts.Arguments,
				License:         contracts.License,
				Language:        contracts.Language,
				Compiler:        contracts.Compiler,
				Optimize:        contracts.Optimize,
				OptimizeRuns:    contracts.OptimizeRuns,
				ContractName:    contractName,
				ByteCode:        byteCode,
				ABI:             ABI,
				HasBeenVerified: true,
			}
		}
	}
	return
}

func (v VerifyContractBiz) getInputActor(ctx context.Context, input string) (resp *londobell.ActorState, err error) {
	var inputAddress chain.SmartAddress
	address, err := CheckETHAddress(input)
	if err != nil {
		return
	}
	if address != "" {
		inputAddress = chain.SmartAddress(address)
	} else {
		inputAddress = chain.SmartAddress(input)
	}
	var inputActor *londobell.ActorState
	inputActor, err = v.adapter.Actor(ctx, inputAddress, nil)
	if err != nil {
		return
	}
	if inputActor != nil {
		resp = inputActor
	}
	return
}

//func (v VerifyContractBiz) VerifyContractByTruffle(ctx context.Context, request filscan.VerifyContractRequest) (resp filscan.VerifyContractResponse, err error) {
//	actor, err := v.GetOnChainActor(ctx, request.ContractAddress)
//	if err != nil {
//		return
//	}
//	var actorID string
//	var actorAddress string
//	var ethAddress string
//	var actorInitCode *londobell.InitCode
//	if actor != nil {
//		actorID = actor.ActorID
//		actorAddress = actor.ActorAddress
//		ethAddress = actor.EthAddress
//		actorInitCode = actor.InitCode
//	} else {
//		resp.IsVerified = false
//		return filscan.VerifyContractResponse{}, fmt.Errorf("can't find the input address %s", request.ContractAddress)
//	}
//
//	var onChainByteCode string
//	var initCode string
//	if actorInitCode != nil {
//		onChainByteCode = "0x" + actorInitCode.InitCode[:len(actorInitCode.InitCode)-86]
//		initCode = actorInitCode.InitCode
//	} else {
//		resp.IsVerified = false
//		return
//	}
//
//	var sourceFile []contract.SourceFile
//	if request.SourceFile != nil {
//		for _, file := range request.SourceFile {
//			sourceFile = append(sourceFile, contract.SourceFile{
//				Name:    file.FileName,
//				RawCode: file.SourceCode,
//			})
//		}
//	} else {
//		resp.IsVerified = false
//		return
//	}
//
//	var targetVersion string
//	var versionRegexp = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)
//	matches := versionRegexp.FindStringSubmatch(request.CompileVersion)
//	if len(matches) != 1 {
//		return filscan.VerifyContractResponse{}, fmt.Errorf("can't parse solc version %q", request.CompileVersion)
//	} else {
//		targetVersion = matches[0]
//	}
//
//	truffle := contract.Truffle{
//		Compilers: contract.Compilers{
//			Solc: contract.Solc{
//				Version: targetVersion,
//				Docker:  true,
//				Settings: contract.Settings{
//					Optimizer: contract.Optimizer{
//						Enabled: request.Enabled,
//						Runs:    request.OptimizeRuns,
//					},
//				},
//			},
//		},
//		Config: v.config,
//	}
//	compiledContracts, err := truffle.CompileByTruffle(sourceFile)
//	if err != nil {
//		return
//	}
//	var mainContract *contract.CompiledFile
//	if compiledContracts != nil {
//		for _, compiledContract := range compiledContracts {
//
//			if strings.Contains(compiledContract.Bytecode, onChainByteCode) {
//				mainContract = &contract.CompiledFile{
//					ContractName:     compiledContract.ContractName,
//					ByteCode:         compiledContract.Bytecode,
//					DeployedByteCode: compiledContract.DeployedBytecode,
//					ABI:              compiledContract.Abi,
//					ContractFile: contract.SourceFile{
//						Name:    compiledContract.SourcePath,
//						RawCode: compiledContract.Source,
//					},
//				}
//			}
//		}
//	} else {
//		resp.IsVerified = false
//		return
//	}
//
//	var compiledByteCode string
//	if mainContract != nil {
//		compiledByteCode = mainContract.ByteCode[:len(mainContract.ByteCode)-86]
//	} else {
//		resp.IsVerified = false
//		return
//	}
//
//	if strings.Contains(compiledByteCode, onChainByteCode) {
//		fEVMContract := &po.FEvmContracts{
//			ActorID:         actorID,
//			ActorAddress:    actorAddress,
//			ContractAddress: ethAddress,
//			Arguments:       request.Arguments,
//			License:         request.License,
//			Language:        "Solidity",
//			Compiler:        request.CompileVersion,
//			Enabled:        request.Enabled,
//			OptimizeRuns:    request.OptimizeRuns,
//			Verified:        true,
//			CreatedAt:       time.Now(),
//		}
//		var d []byte
//		d, err = json.Marshal(mainContract.ABI)
//		if err != nil {
//			return
//		}
//
//		abi := string(d)
//		fEVMContractSol := &po.FEvmContractSols{
//			ActorID:      actorID,
//			FileName:     mainContract.ContractFile.Name,
//			Source:       mainContract.ContractFile.RawCode,
//			Size:         int64(types.SizeOfStdString(mainContract.ContractFile.RawCode)),
//			ContractName: mainContract.ContractName,
//			ByteCode:     initCode,
//			ABI:          abi,
//			CreatedAt:    time.Now(),
//		}
//
//		var tx *gorm.DB
//		tx, err = v.gorm.DB(ctx)
//		if err != nil {
//			return
//		}
//		tx.Begin()
//		err = v.contractsDB.SaveFEvmContracts(ctx, fEVMContract)
//		if err != nil {
//			tx.Rollback()
//			return
//		}
//		err = v.contractsDB.SaveFEvmContractSols(ctx, fEVMContractSol)
//		if err != nil {
//			tx.Rollback()
//			return
//		}
//		tx.Commit()
//
//		resp = filscan.VerifyContractResponse{
//			IsVerified: true,
//			CompiledFile: &filscan.CompiledFile{
//				ActorID:         actorID,
//				ActorAddress:    actorAddress,
//				ContractAddress: ethAddress,
//				Arguments:       request.Arguments,
//				License:         request.License,
//				Language:        "Solidity",
//				Compiler:        request.CompileVersion,
//				Enabled:        request.Enabled,
//				OptimizeRuns:    request.OptimizeRuns,
//				ContractName:    mainContract.ContractName,
//				ByteCode:        initCode,
//				ABI:             abi,
//				LocalByteCode:   initCode,
//			},
//		}
//	} else {
//		resp.IsVerified = false
//		return
//	}
//
//	return
//}
// 6080604052604051620017d4380380620017d4833981018060405260c08110156200002957600080fd5b8101908080516401000000008111156200004257600080fd5b828101905060208101848111156200005957600080fd5b81518560018202830111640100000000821117156200007757600080fd5b505092919060200180516401000000008111156200009457600080fd5b82810190506020810184811115620000ab57600080fd5b8151856001820283011164010000000082111715620000c957600080fd5b50509291906020018051906020019092919080519060200190929190805190602001909291908051906020019092919050505085600390805190602001906200011492919062000421565b5084600490805190602001906200012d92919062000421565b5083600560006101000a81548160ff021916908360ff160217905550620001648184620001b8640100000000026401000000009004565b8173ffffffffffffffffffffffffffffffffffffffff166108fc349081150290604051600060405180830381858888f19350505050158015620001ab573d6000803e3d6000fd5b50505050505050620004d0565b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff16141515156200025e576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601f8152602001807f45524332303a206d696e7420746f20746865207a65726f20616464726573730081525060200191505060405180910390fd5b620002838160025462000396640100000000026200105b179091906401000000009004565b600281905550620002ea816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205462000396640100000000026200105b179091906401000000009004565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a35050565b600080828401905083811015151562000417576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f106200046457805160ff191683800117855562000495565b8280016001018555821562000495579182015b828111156200049457825182559160200191906001019062000477565b5b509050620004a49190620004a8565b5090565b620004cd91905b80821115620004c9576000816000905550600101620004af565b5090565b90565b6112f480620004e06000396000f3fe6080604052600436106100ba576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806306fdde03146100bf578063095ea7b31461014f57806318160ddd146101c257806323b872dd146101ed578063313ce5671461028057806339509351146102b157806342966c681461032457806370a082311461035f57806395d89b41146103c4578063a457c2d714610454578063a9059cbb146104c7578063dd62ed3e1461053a575b600080fd5b3480156100cb57600080fd5b506100d46105bf565b6040518080602001828103825283818151815260200191508051906020019080838360005b838110156101145780820151818401526020810190506100f9565b50505050905090810190601f1680156101415780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b34801561015b57600080fd5b506101a86004803603604081101561017257600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610661565b604051808215151515815260200191505060405180910390f35b3480156101ce57600080fd5b506101d7610678565b6040518082815260200191505060405180910390f35b3480156101f957600080fd5b506102666004803603606081101561021057600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610682565b604051808215151515815260200191505060405180910390f35b34801561028c57600080fd5b50610295610733565b604051808260ff1660ff16815260200191505060405180910390f35b3480156102bd57600080fd5b5061030a600480360360408110156102d457600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019092919050505061074a565b604051808215151515815260200191505060405180910390f35b34801561033057600080fd5b5061035d6004803603602081101561034757600080fd5b81019080803590602001909291905050506107ef565b005b34801561036b57600080fd5b506103ae6004803603602081101561038257600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506107fc565b6040518082815260200191505060405180910390f35b3480156103d057600080fd5b506103d9610844565b6040518080602001828103825283818151815260200191508051906020019080838360005b838110156104195780820151818401526020810190506103fe565b50505050905090810190601f1680156104465780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b34801561046057600080fd5b506104ad6004803603604081101561047757600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506108e6565b604051808215151515815260200191505060405180910390f35b3480156104d357600080fd5b50610520600480360360408110156104ea57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019092919050505061098b565b604051808215151515815260200191505060405180910390f35b34801561054657600080fd5b506105a96004803603604081101561055d57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506109a2565b6040518082815260200191505060405180910390f35b606060038054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156106575780601f1061062c57610100808354040283529160200191610657565b820191906000526020600020905b81548152906001019060200180831161063a57829003601f168201915b5050505050905090565b600061066e338484610a29565b6001905092915050565b6000600254905090565b600061068f848484610caa565b610728843361072385600160008a73ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b610a29565b600190509392505050565b6000600560009054906101000a900460ff16905090565b60006107e533846107e085600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008973ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205461105b90919063ffffffff16565b610a29565b6001905092915050565b6107f933826110e5565b50565b60008060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b606060048054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156108dc5780601f106108b1576101008083540402835291602001916108dc565b820191906000526020600020905b8154815290600101906020018083116108bf57829003601f168201915b5050505050905090565b6000610981338461097c85600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008973ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b610a29565b6001905092915050565b6000610998338484610caa565b6001905092915050565b6000600160008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054905092915050565b600073ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff1614151515610af4576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260248152602001807f45524332303a20617070726f76652066726f6d20746865207a65726f2061646481526020017f726573730000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff1614151515610bbf576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260228152602001807f45524332303a20617070726f766520746f20746865207a65726f20616464726581526020017f737300000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b80600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff167f8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925836040518082815260200191505060405180910390a3505050565b600073ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff1614151515610d75576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260258152602001807f45524332303a207472616e736665722066726f6d20746865207a65726f20616481526020017f647265737300000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff1614151515610e40576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260238152602001807f45524332303a207472616e7366657220746f20746865207a65726f206164647281526020017f657373000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b610e91816000808673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b6000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610f24816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205461105b90919063ffffffff16565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a3505050565b600082821115151561104a576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f536166654d6174683a207375627472616374696f6e206f766572666c6f77000081525060200191505060405180910390fd5b600082840390508091505092915050565b60008082840190508381101515156110db576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff16141515156111b0576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260218152602001807f45524332303a206275726e2066726f6d20746865207a65726f2061646472657381526020017f730000000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b6111c581600254610fd090919063ffffffff16565b60028190555061121c816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a3505056fea165627a7a72305820a4b3fd67b63fdebf9fe88089291d019a8ca4d09147560dd40a81b8b18a3c9d4a002900000000000000000000000000000000000000000000000000000000000000c000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000012000000000000000000000000000000000000314dc6448d9338c15b0a0000000000000000000000000000000063abe950294fcfb935ba1e10f359359939d5e4ff0000000000000000000000008de4432f4f51b4351a23f5ccbf9e7245ccdc2041000000000000000000000000000000000000000000000000000000000000000c46494c45444f4745544553540000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c46494c45444f4745544553540000000000000000000000000000000000000000
// 6080604052604051620017d4380380620017d4833981018060405260c08110156200002957600080fd5b8101908080516401000000008111156200004257600080fd5b828101905060208101848111156200005957600080fd5b81518560018202830111640100000000821117156200007757600080fd5b505092919060200180516401000000008111156200009457600080fd5b82810190506020810184811115620000ab57600080fd5b8151856001820283011164010000000082111715620000c957600080fd5b50509291906020018051906020019092919080519060200190929190805190602001909291908051906020019092919050505085600390805190602001906200011492919062000421565b5084600490805190602001906200012d92919062000421565b5083600560006101000a81548160ff021916908360ff160217905550620001648184620001b8640100000000026401000000009004565b8173ffffffffffffffffffffffffffffffffffffffff166108fc349081150290604051600060405180830381858888f19350505050158015620001ab573d6000803e3d6000fd5b50505050505050620004d0565b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff16141515156200025e576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601f8152602001807f45524332303a206d696e7420746f20746865207a65726f20616464726573730081525060200191505060405180910390fd5b620002838160025462000396640100000000026200105b179091906401000000009004565b600281905550620002ea816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205462000396640100000000026200105b179091906401000000009004565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a35050565b600080828401905083811015151562000417576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f106200046457805160ff191683800117855562000495565b8280016001018555821562000495579182015b828111156200049457825182559160200191906001019062000477565b5b509050620004a49190620004a8565b5090565b620004cd91905b80821115620004c9576000816000905550600101620004af565b5090565b90565b6112f480620004e06000396000f3fe6080604052600436106100ba576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806306fdde03146100bf578063095ea7b31461014f57806318160ddd146101c257806323b872dd146101ed578063313ce5671461028057806339509351146102b157806342966c681461032457806370a082311461035f57806395d89b41146103c4578063a457c2d714610454578063a9059cbb146104c7578063dd62ed3e1461053a575b600080fd5b3480156100cb57600080fd5b506100d46105bf565b6040518080602001828103825283818151815260200191508051906020019080838360005b838110156101145780820151818401526020810190506100f9565b50505050905090810190601f1680156101415780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b34801561015b57600080fd5b506101a86004803603604081101561017257600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610661565b604051808215151515815260200191505060405180910390f35b3480156101ce57600080fd5b506101d7610678565b6040518082815260200191505060405180910390f35b3480156101f957600080fd5b506102666004803603606081101561021057600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610682565b604051808215151515815260200191505060405180910390f35b34801561028c57600080fd5b50610295610733565b604051808260ff1660ff16815260200191505060405180910390f35b3480156102bd57600080fd5b5061030a600480360360408110156102d457600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019092919050505061074a565b604051808215151515815260200191505060405180910390f35b34801561033057600080fd5b5061035d6004803603602081101561034757600080fd5b81019080803590602001909291905050506107ef565b005b34801561036b57600080fd5b506103ae6004803603602081101561038257600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506107fc565b6040518082815260200191505060405180910390f35b3480156103d057600080fd5b506103d9610844565b6040518080602001828103825283818151815260200191508051906020019080838360005b838110156104195780820151818401526020810190506103fe565b50505050905090810190601f1680156104465780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b34801561046057600080fd5b506104ad6004803603604081101561047757600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506108e6565b604051808215151515815260200191505060405180910390f35b3480156104d357600080fd5b50610520600480360360408110156104ea57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019092919050505061098b565b604051808215151515815260200191505060405180910390f35b34801561054657600080fd5b506105a96004803603604081101561055d57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506109a2565b6040518082815260200191505060405180910390f35b606060038054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156106575780601f1061062c57610100808354040283529160200191610657565b820191906000526020600020905b81548152906001019060200180831161063a57829003601f168201915b5050505050905090565b600061066e338484610a29565b6001905092915050565b6000600254905090565b600061068f848484610caa565b610728843361072385600160008a73ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b610a29565b600190509392505050565b6000600560009054906101000a900460ff16905090565b60006107e533846107e085600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008973ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205461105b90919063ffffffff16565b610a29565b6001905092915050565b6107f933826110e5565b50565b60008060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b606060048054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156108dc5780601f106108b1576101008083540402835291602001916108dc565b820191906000526020600020905b8154815290600101906020018083116108bf57829003601f168201915b5050505050905090565b6000610981338461097c85600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008973ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b610a29565b6001905092915050565b6000610998338484610caa565b6001905092915050565b6000600160008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054905092915050565b600073ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff1614151515610af4576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260248152602001807f45524332303a20617070726f76652066726f6d20746865207a65726f2061646481526020017f726573730000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff1614151515610bbf576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260228152602001807f45524332303a20617070726f766520746f20746865207a65726f20616464726581526020017f737300000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b80600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff167f8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925836040518082815260200191505060405180910390a3505050565b600073ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff1614151515610d75576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260258152602001807f45524332303a207472616e736665722066726f6d20746865207a65726f20616481526020017f647265737300000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff1614151515610e40576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260238152602001807f45524332303a207472616e7366657220746f20746865207a65726f206164647281526020017f657373000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b610e91816000808673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b6000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610f24816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205461105b90919063ffffffff16565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a3505050565b600082821115151561104a576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f536166654d6174683a207375627472616374696f6e206f766572666c6f77000081525060200191505060405180910390fd5b600082840390508091505092915050565b60008082840190508381101515156110db576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff16141515156111b0576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260218152602001807f45524332303a206275726e2066726f6d20746865207a65726f2061646472657381526020017f730000000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b6111c581600254610fd090919063ffffffff16565b60028190555061121c816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fd090919063ffffffff16565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a3505056fea165627a7a723058208322eeb4f07e772b28181715d5d36d09635b5af1be59a27579492520a22d1abe0029
