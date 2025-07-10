package contract

import "strings"

func CompareByteCodeWithTarget(compiledContracts []*CompiledFile, initCode string, target string) (comparedContracts []*CompiledFile) {
	initCodeNoCBOR := RemoveCBORCode(initCode)
	if target != "" {
		for _, compiledContract := range compiledContracts {
			if compiledContract.ContractName == target {
				byteCodeNoCBOR := RemoveCBORCode(compiledContract.ByteCode)
				if len(initCodeNoCBOR) >= len(byteCodeNoCBOR) && byteCodeNoCBOR != compiledContract.ByteCode {
					argumentsCode := initCodeNoCBOR[len(byteCodeNoCBOR):]
					compareCode := initCodeNoCBOR[:len(byteCodeNoCBOR)]
					compareCode, byteCodeNoCBOR = RemovePlaceholderCode(compareCode, byteCodeNoCBOR)
					if compareCode == byteCodeNoCBOR {
						compiledContract.Arguments = argumentsCode
						compiledContract.IsMainContract = true
					}
				}
				comparedContracts = append(comparedContracts, compiledContract)
				break
			}
		}
	} else {
		for _, compiledContract := range compiledContracts {
			byteCodeNoCBOR := RemoveCBORCode(compiledContract.ByteCode)
			if len(initCodeNoCBOR) >= len(byteCodeNoCBOR) && byteCodeNoCBOR != compiledContract.ByteCode {
				argumentsCode := initCodeNoCBOR[len(byteCodeNoCBOR):]
				compareCode := initCodeNoCBOR[:len(byteCodeNoCBOR)]
				compareCode, byteCodeNoCBOR = RemovePlaceholderCode(compareCode, byteCodeNoCBOR)
				if compareCode == byteCodeNoCBOR {
					compiledContract.Arguments = argumentsCode
					compiledContract.IsMainContract = true
				}
			}
			comparedContracts = append(comparedContracts, compiledContract)
		}
	}
	return
}

func RemoveCBORCode(withCbor string) (withoutCbor string) {
	input := withCbor
	const endStr = "64736f6c6343"
	const lenMid = 64
	var endPos int
	var cborCode []string
	for {
		endPos = strings.Index(input, endStr)
		if endPos < lenMid {
			break
		}
		midStr := input[endPos-lenMid : endPos]
		cborCode = append(cborCode, midStr)
		input = input[endPos+len(endStr):]
	}
	if cborCode != nil {
		for _, cbor := range cborCode {
			withCbor = strings.ReplaceAll(withCbor, cbor, "")
		}
	}
	withoutCbor = withCbor

	return
}

func CompareByteCodeOldVersionWithTarget(compiledContracts []*CompiledFile, initCode string, target string) (comparedContracts []*CompiledFile) {
	const lenCbor = 68
	if target != "" {
		for _, compiledContract := range compiledContracts {
			if compiledContract.ContractName == target {
				localCode := compiledContract.ByteCode
				argumentsCode := initCode[len(localCode):]
				compareCode := initCode[:len(localCode)]
				compareCode, localCode = RemovePlaceholderCode(compareCode, localCode)
				if len(compareCode) > lenCbor && len(localCode) > lenCbor && compareCode[:len(compareCode)-lenCbor] == localCode[:len(localCode)-lenCbor] {
					compiledContract.Arguments = argumentsCode
					compiledContract.IsMainContract = true
				}
				comparedContracts = append(comparedContracts, compiledContract)
				break
			}
		}
	} else {
		for _, compiledContract := range compiledContracts {
			localCode := compiledContract.ByteCode
			argumentsCode := initCode[len(localCode):]
			compareCode := initCode[:len(localCode)]
			compareCode, localCode = RemovePlaceholderCode(compareCode, localCode)
			if len(compareCode) > lenCbor && len(localCode) > lenCbor && compareCode[:len(compareCode)-lenCbor] == localCode[:len(localCode)-lenCbor] {
				compiledContract.Arguments = argumentsCode
				compiledContract.IsMainContract = true
			}
			comparedContracts = append(comparedContracts, compiledContract)
		}
	}
	return
}

func RemovePlaceholderCode(initCode string, byteCode string) (initCodeResult string, byteCodeResult string) {
	const prefix = "__$"
	const suffix = "$__"
	for {
		prefixIndex := strings.Index(byteCode, prefix)
		suffixIndex := strings.Index(byteCode, suffix) + len(suffix)
		if prefixIndex > 0 && suffixIndex > 0 {
			byteCode = byteCode[:prefixIndex] + byteCode[suffixIndex:]
			initCode = initCode[:prefixIndex] + initCode[suffixIndex:]
		}
		if prefixIndex == -1 || suffixIndex == -1 {
			break
		}
	}
	initCodeResult = initCode
	byteCodeResult = byteCode
	return
}
