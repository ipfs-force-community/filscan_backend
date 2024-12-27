package providers

const (
	Filfox   = "filfox"
	Opengate = "opengate"
)

type FNSProvider struct {
	Alias       string
	Contract    string
	LOGO        string
	Name        string
	MainSite    string
	TwitterLink string
}

var mapping = map[string]FNSProvider{
	"0x45d9d6408d5159a379924cf423cb7e15C00fA81f": {
		Alias:       Filfox,
		Name:        "FNS DAO",
		Contract:    "0x45d9d6408d5159a379924cf423cb7e15C00fA81f",
		LOGO:        "https://filscan-v2.oss-cn-hongkong.aliyuncs.com/fvm_manage/images/FNS%20DAO.jpeg",
		MainSite:    "https://app.fns.space/",
		TwitterLink: "https://twitter.com/FNS_SPACE",
	},
	"0xb8d7ca6a3253c418e52087693ca688d3257d70d1": {
		Alias:       Opengate,
		Name:        "FNS Filecoin Name service",
		Contract:    "0xb8d7ca6a3253c418e52087693ca688d3257d70d1",
		LOGO:        "https://filscan-v2.oss-cn-hongkong.aliyuncs.com/fvm_manage/images/opengate%20fns.jpeg",
		MainSite:    "https://opengatefns.com/",
		TwitterLink: "https://twitter.com/OpenGateNFT",
	},
}

func ToContract(alias string) string {
	m := map[string]string{}
	for k, v := range mapping {
		m[v.Alias] = k
	}
	return m[alias]
}

func GetLogo(contract string) string {
	if v, ok := mapping[contract]; ok {
		return v.LOGO
	}
	return ""
}

func ToAlias(contract string) string {
	return mapping[contract].Alias
}

func GetProvider(alias string) FNSProvider {
	return mapping[ToContract(alias)]
}
