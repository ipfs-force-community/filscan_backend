package assembler

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
)

type VerifyContact struct {
}

func (v VerifyContact) VerifiedContractsToCompiledFile(contract *po.FEvmContracts, sols []*po.FEvmContractSols) (result *filscan.VerifiedContractResponse) {
	var contractName, byteCode, ABI string
	solFiles := make(map[string]string)
	for _, sol := range sols {
		if sol.IsMainContract {
			contractName = sol.ContractName
			byteCode = sol.ByteCode
			ABI = sol.ABI
		}
		solFiles[sol.FileName] = sol.Source
	}
	var sourceFiles []*filscan.SourceFile
	for key, value := range solFiles {
		sourceFiles = append(sourceFiles, &filscan.SourceFile{
			FileName:   key,
			SourceCode: value,
		})
	}

	result = &filscan.VerifiedContractResponse{
		CompiledFile: &filscan.CompiledFile{
			ActorID:         contract.ActorID,
			ActorAddress:    contract.ActorAddress,
			ContractAddress: contract.ContractAddress,
			Arguments:       contract.Arguments,
			License:         contract.License,
			Language:        contract.Language,
			Compiler:        contract.Compiler,
			Optimize:        contract.Optimize,
			OptimizeRuns:    contract.OptimizeRuns,
			ContractName:    contractName,
			ByteCode:        byteCode,
			ABI:             ABI,
		},
		SourceFile: sourceFiles,
	}

	return
}

func (v VerifyContact) VerifiedContractsToCompiledFileList(contracts []*bo.VerifiedContracts) (result []*filscan.CompiledFile) {
	for _, contract := range contracts {
		result = append(result, &filscan.CompiledFile{
			ActorID:         contract.ActorID,
			ActorAddress:    contract.ActorAddress,
			ContractAddress: contract.ContractAddress,
			License:         contract.License,
			Language:        contract.Language,
			Compiler:        contract.Compiler,
			Optimize:        contract.Optimize,
			OptimizeRuns:    contract.OptimizeRuns,
			ContractName:    contract.ContractName,
		})
	}
	return
}
