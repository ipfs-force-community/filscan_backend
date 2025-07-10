package contract

import "time"

type AbiEntry struct {
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	Inputs          []AbiParameter `json:"inputs"`
	Outputs         []AbiParameter `json:"outputs"`
	Constant        bool           `json:"constant"`
	Payable         bool           `json:"payable"`
	StateMutability string         `json:"stateMutability"`
}

type AbiParameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Contract struct {
	Abi              []AbiEntry `json:"abi"`
	Bytecode         string     `json:"bytecode"`
	DeployedBytecode string     `json:"deployedBytecode"`
}

type ContractInfo struct {
	ContractName string `json:"contractName"`
	Abi          []struct {
		Inputs []struct {
			InternalType string `json:"internalType"`
			Name         string `json:"name"`
			Type         string `json:"type"`
		} `json:"inputs"`
		Name    string `json:"name"`
		Outputs []struct {
			InternalType string `json:"internalType"`
			Name         string `json:"name"`
			Type         string `json:"type"`
		} `json:"outputs"`
		StateMutability string `json:"stateMutability"`
		Type            string `json:"type"`
	} `json:"abi"`
	Metadata            string `json:"metadata"`
	Bytecode            string `json:"bytecode"`
	DeployedBytecode    string `json:"deployedBytecode"`
	ImmutableReferences struct {
	} `json:"immutableReferences"`
	GeneratedSources         []interface{} `json:"generatedSources"`
	DeployedGeneratedSources []struct {
		Ast struct {
			NodeType   string `json:"nodeType"`
			Src        string `json:"src"`
			Statements []struct {
				Body struct {
					NodeType   string `json:"nodeType"`
					Src        string `json:"src"`
					Statements []struct {
						NodeType string `json:"nodeType"`
						Src      string `json:"src"`
						Value    struct {
							Name      string `json:"name,omitempty"`
							NodeType  string `json:"nodeType"`
							Src       string `json:"src"`
							Arguments []struct {
								Name     string `json:"name,omitempty"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
								Kind     string `json:"kind,omitempty"`
								Type     string `json:"type,omitempty"`
								Value    string `json:"value,omitempty"`
							} `json:"arguments,omitempty"`
							FunctionName struct {
								Name     string `json:"name"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
							} `json:"functionName,omitempty"`
						} `json:"value,omitempty"`
						VariableNames []struct {
							Name     string `json:"name"`
							NodeType string `json:"nodeType"`
							Src      string `json:"src"`
						} `json:"variableNames,omitempty"`
						Expression struct {
							Arguments []struct {
								Name      string `json:"name,omitempty"`
								NodeType  string `json:"nodeType"`
								Src       string `json:"src"`
								Arguments []struct {
									Name     string `json:"name,omitempty"`
									NodeType string `json:"nodeType"`
									Src      string `json:"src"`
									Kind     string `json:"kind,omitempty"`
									Type     string `json:"type,omitempty"`
									Value    string `json:"value,omitempty"`
								} `json:"arguments,omitempty"`
								FunctionName struct {
									Name     string `json:"name"`
									NodeType string `json:"nodeType"`
									Src      string `json:"src"`
								} `json:"functionName,omitempty"`
								Kind  string `json:"kind,omitempty"`
								Type  string `json:"type,omitempty"`
								Value string `json:"value,omitempty"`
							} `json:"arguments"`
							FunctionName struct {
								Name     string `json:"name"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
							} `json:"functionName"`
							NodeType string `json:"nodeType"`
							Src      string `json:"src"`
						} `json:"expression,omitempty"`
						Body struct {
							NodeType   string `json:"nodeType"`
							Src        string `json:"src"`
							Statements []struct {
								Expression struct {
									Arguments []struct {
										Kind     string `json:"kind"`
										NodeType string `json:"nodeType"`
										Src      string `json:"src"`
										Type     string `json:"type"`
										Value    string `json:"value"`
									} `json:"arguments"`
									FunctionName struct {
										Name     string `json:"name"`
										NodeType string `json:"nodeType"`
										Src      string `json:"src"`
									} `json:"functionName"`
									NodeType string `json:"nodeType"`
									Src      string `json:"src"`
								} `json:"expression"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
							} `json:"statements"`
						} `json:"body,omitempty"`
						Condition struct {
							Arguments []struct {
								Arguments []struct {
									Name      string `json:"name,omitempty"`
									NodeType  string `json:"nodeType"`
									Src       string `json:"src"`
									Arguments []struct {
										Name     string `json:"name"`
										NodeType string `json:"nodeType"`
										Src      string `json:"src"`
									} `json:"arguments,omitempty"`
									FunctionName struct {
										Name     string `json:"name"`
										NodeType string `json:"nodeType"`
										Src      string `json:"src"`
									} `json:"functionName,omitempty"`
								} `json:"arguments,omitempty"`
								FunctionName struct {
									Name     string `json:"name"`
									NodeType string `json:"nodeType"`
									Src      string `json:"src"`
								} `json:"functionName,omitempty"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
								Kind     string `json:"kind,omitempty"`
								Type     string `json:"type,omitempty"`
								Value    string `json:"value,omitempty"`
							} `json:"arguments"`
							FunctionName struct {
								Name     string `json:"name"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
							} `json:"functionName"`
							NodeType string `json:"nodeType"`
							Src      string `json:"src"`
						} `json:"condition,omitempty"`
						Statements []struct {
							NodeType string `json:"nodeType"`
							Src      string `json:"src"`
							Value    struct {
								Kind      string `json:"kind,omitempty"`
								NodeType  string `json:"nodeType"`
								Src       string `json:"src"`
								Type      string `json:"type,omitempty"`
								Value     string `json:"value,omitempty"`
								Arguments []struct {
									Arguments []struct {
										Name     string `json:"name"`
										NodeType string `json:"nodeType"`
										Src      string `json:"src"`
									} `json:"arguments,omitempty"`
									FunctionName struct {
										Name     string `json:"name"`
										NodeType string `json:"nodeType"`
										Src      string `json:"src"`
									} `json:"functionName,omitempty"`
									NodeType string `json:"nodeType"`
									Src      string `json:"src"`
									Name     string `json:"name,omitempty"`
								} `json:"arguments,omitempty"`
								FunctionName struct {
									Name     string `json:"name"`
									NodeType string `json:"nodeType"`
									Src      string `json:"src"`
								} `json:"functionName,omitempty"`
							} `json:"value"`
							Variables []struct {
								Name     string `json:"name"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
								Type     string `json:"type"`
							} `json:"variables,omitempty"`
							VariableNames []struct {
								Name     string `json:"name"`
								NodeType string `json:"nodeType"`
								Src      string `json:"src"`
							} `json:"variableNames,omitempty"`
						} `json:"statements,omitempty"`
					} `json:"statements"`
				} `json:"body"`
				Name       string `json:"name"`
				NodeType   string `json:"nodeType"`
				Parameters []struct {
					Name     string `json:"name"`
					NodeType string `json:"nodeType"`
					Src      string `json:"src"`
					Type     string `json:"type"`
				} `json:"parameters,omitempty"`
				ReturnVariables []struct {
					Name     string `json:"name"`
					NodeType string `json:"nodeType"`
					Src      string `json:"src"`
					Type     string `json:"type"`
				} `json:"returnVariables,omitempty"`
				Src string `json:"src"`
			} `json:"statements"`
		} `json:"ast"`
		Contents string `json:"contents"`
		Id       int    `json:"id"`
		Language string `json:"language"`
		Name     string `json:"name"`
	} `json:"deployedGeneratedSources"`
	SourceMap         string `json:"sourceMap"`
	DeployedSourceMap string `json:"deployedSourceMap"`
	Source            string `json:"source"`
	SourcePath        string `json:"sourcePath"`
	Ast               struct {
		AbsolutePath    string `json:"absolutePath"`
		ExportedSymbols struct {
			Storage []int `json:"Storage"`
		} `json:"exportedSymbols"`
		Id       int    `json:"id"`
		License  string `json:"license"`
		NodeType string `json:"nodeType"`
		Nodes    []struct {
			Id                   int           `json:"id"`
			Literals             []string      `json:"literals,omitempty"`
			NodeType             string        `json:"nodeType"`
			Src                  string        `json:"src"`
			Abstract             bool          `json:"abstract,omitempty"`
			BaseContracts        []interface{} `json:"baseContracts,omitempty"`
			CanonicalName        string        `json:"canonicalName,omitempty"`
			ContractDependencies []interface{} `json:"contractDependencies,omitempty"`
			ContractKind         string        `json:"contractKind,omitempty"`
			Documentation        struct {
				Id       int    `json:"id"`
				NodeType string `json:"nodeType"`
				Src      string `json:"src"`
				Text     string `json:"text"`
			} `json:"documentation,omitempty"`
			FullyImplemented        bool   `json:"fullyImplemented,omitempty"`
			LinearizedBaseContracts []int  `json:"linearizedBaseContracts,omitempty"`
			Name                    string `json:"name,omitempty"`
			NameLocation            string `json:"nameLocation,omitempty"`
			Nodes                   []struct {
				Constant         bool   `json:"constant,omitempty"`
				Id               int    `json:"id"`
				Mutability       string `json:"mutability,omitempty"`
				Name             string `json:"name"`
				NameLocation     string `json:"nameLocation"`
				NodeType         string `json:"nodeType"`
				Scope            int    `json:"scope"`
				Src              string `json:"src"`
				StateVariable    bool   `json:"stateVariable,omitempty"`
				StorageLocation  string `json:"storageLocation,omitempty"`
				TypeDescriptions struct {
					TypeIdentifier string `json:"typeIdentifier"`
					TypeString     string `json:"typeString"`
				} `json:"typeDescriptions,omitempty"`
				TypeName struct {
					Id               int    `json:"id"`
					Name             string `json:"name"`
					NodeType         string `json:"nodeType"`
					Src              string `json:"src"`
					TypeDescriptions struct {
						TypeIdentifier string `json:"typeIdentifier"`
						TypeString     string `json:"typeString"`
					} `json:"typeDescriptions"`
				} `json:"typeName,omitempty"`
				Visibility string `json:"visibility"`
				Body       struct {
					Id         int    `json:"id"`
					NodeType   string `json:"nodeType"`
					Src        string `json:"src"`
					Statements []struct {
						Expression struct {
							Id              int  `json:"id"`
							IsConstant      bool `json:"isConstant,omitempty"`
							IsLValue        bool `json:"isLValue,omitempty"`
							IsPure          bool `json:"isPure,omitempty"`
							LValueRequested bool `json:"lValueRequested,omitempty"`
							LeftHandSide    struct {
								Id                     int           `json:"id"`
								Name                   string        `json:"name"`
								NodeType               string        `json:"nodeType"`
								OverloadedDeclarations []interface{} `json:"overloadedDeclarations"`
								ReferencedDeclaration  int           `json:"referencedDeclaration"`
								Src                    string        `json:"src"`
								TypeDescriptions       struct {
									TypeIdentifier string `json:"typeIdentifier"`
									TypeString     string `json:"typeString"`
								} `json:"typeDescriptions"`
							} `json:"leftHandSide,omitempty"`
							NodeType      string `json:"nodeType"`
							Operator      string `json:"operator,omitempty"`
							RightHandSide struct {
								Id                     int           `json:"id"`
								Name                   string        `json:"name"`
								NodeType               string        `json:"nodeType"`
								OverloadedDeclarations []interface{} `json:"overloadedDeclarations"`
								ReferencedDeclaration  int           `json:"referencedDeclaration"`
								Src                    string        `json:"src"`
								TypeDescriptions       struct {
									TypeIdentifier string `json:"typeIdentifier"`
									TypeString     string `json:"typeString"`
								} `json:"typeDescriptions"`
							} `json:"rightHandSide,omitempty"`
							Src              string `json:"src"`
							TypeDescriptions struct {
								TypeIdentifier string `json:"typeIdentifier"`
								TypeString     string `json:"typeString"`
							} `json:"typeDescriptions"`
							Name                   string        `json:"name,omitempty"`
							OverloadedDeclarations []interface{} `json:"overloadedDeclarations,omitempty"`
							ReferencedDeclaration  int           `json:"referencedDeclaration,omitempty"`
						} `json:"expression"`
						Id                       int    `json:"id"`
						NodeType                 string `json:"nodeType"`
						Src                      string `json:"src"`
						FunctionReturnParameters int    `json:"functionReturnParameters,omitempty"`
					} `json:"statements"`
				} `json:"body,omitempty"`
				Documentation struct {
					Id       int    `json:"id"`
					NodeType string `json:"nodeType"`
					Src      string `json:"src"`
					Text     string `json:"text"`
				} `json:"documentation,omitempty"`
				FunctionSelector string        `json:"functionSelector,omitempty"`
				Implemented      bool          `json:"implemented,omitempty"`
				Kind             string        `json:"kind,omitempty"`
				Modifiers        []interface{} `json:"modifiers,omitempty"`
				Parameters       struct {
					Id         int    `json:"id"`
					NodeType   string `json:"nodeType"`
					Parameters []struct {
						Constant         bool   `json:"constant"`
						Id               int    `json:"id"`
						Mutability       string `json:"mutability"`
						Name             string `json:"name"`
						NameLocation     string `json:"nameLocation"`
						NodeType         string `json:"nodeType"`
						Scope            int    `json:"scope"`
						Src              string `json:"src"`
						StateVariable    bool   `json:"stateVariable"`
						StorageLocation  string `json:"storageLocation"`
						TypeDescriptions struct {
							TypeIdentifier string `json:"typeIdentifier"`
							TypeString     string `json:"typeString"`
						} `json:"typeDescriptions"`
						TypeName struct {
							Id               int    `json:"id"`
							Name             string `json:"name"`
							NodeType         string `json:"nodeType"`
							Src              string `json:"src"`
							TypeDescriptions struct {
								TypeIdentifier string `json:"typeIdentifier"`
								TypeString     string `json:"typeString"`
							} `json:"typeDescriptions"`
						} `json:"typeName"`
						Visibility string `json:"visibility"`
					} `json:"parameters"`
					Src string `json:"src"`
				} `json:"parameters,omitempty"`
				ReturnParameters struct {
					Id         int    `json:"id"`
					NodeType   string `json:"nodeType"`
					Parameters []struct {
						Constant         bool   `json:"constant"`
						Id               int    `json:"id"`
						Mutability       string `json:"mutability"`
						Name             string `json:"name"`
						NameLocation     string `json:"nameLocation"`
						NodeType         string `json:"nodeType"`
						Scope            int    `json:"scope"`
						Src              string `json:"src"`
						StateVariable    bool   `json:"stateVariable"`
						StorageLocation  string `json:"storageLocation"`
						TypeDescriptions struct {
							TypeIdentifier string `json:"typeIdentifier"`
							TypeString     string `json:"typeString"`
						} `json:"typeDescriptions"`
						TypeName struct {
							Id               int    `json:"id"`
							Name             string `json:"name"`
							NodeType         string `json:"nodeType"`
							Src              string `json:"src"`
							TypeDescriptions struct {
								TypeIdentifier string `json:"typeIdentifier"`
								TypeString     string `json:"typeString"`
							} `json:"typeDescriptions"`
						} `json:"typeName"`
						Visibility string `json:"visibility"`
					} `json:"parameters"`
					Src string `json:"src"`
				} `json:"returnParameters,omitempty"`
				StateMutability string `json:"stateMutability,omitempty"`
				Virtual         bool   `json:"virtual,omitempty"`
			} `json:"nodes,omitempty"`
			Scope      int           `json:"scope,omitempty"`
			UsedErrors []interface{} `json:"usedErrors,omitempty"`
		} `json:"nodes"`
		Src string `json:"src"`
	} `json:"ast"`
	Compiler struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"compiler"`
	Networks struct {
	} `json:"networks"`
	SchemaVersion string    `json:"schemaVersion"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Devdoc        struct {
		CustomDevRunScript string `json:"custom:dev-run-script"`
		Details            string `json:"details"`
		Kind               string `json:"kind"`
		Methods            struct {
			Retrieve struct {
				Details string `json:"details"`
				Returns struct {
					Field1 string `json:"_0"`
				} `json:"returns"`
			} `json:"retrieve()"`
			StoreUint256 struct {
				Details string `json:"details"`
				Params  struct {
					Num string `json:"num"`
				} `json:"params"`
			} `json:"store(uint256)"`
		} `json:"methods"`
		Title   string `json:"title"`
		Version int    `json:"version"`
	} `json:"devdoc"`
	Userdoc struct {
		Kind    string `json:"kind"`
		Methods struct {
		} `json:"methods"`
		Version int `json:"version"`
	} `json:"userdoc"`
}
