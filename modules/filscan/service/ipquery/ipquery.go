package ipquery

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/gozelle/pointer"
	"strings"
)

func NewIpQuery() *IpQuery {
	return &IpQuery{query: resty.New()}
}

type IpQuery struct {
	query *resty.Client
}

func (iq IpQuery) Query(ip string) (res *QueryIP, err error) {
	
	r, err := iq.query.R().Get(fmt.Sprintf("https://ipapi.co/%s/json", ip))
	if err != nil {
		return
	}
	res = new(QueryIP)
	err = json.Unmarshal(r.Body(), res)
	if err != nil {
		return
	}
	
	if res.Country == "Hong Kong" {
		res.Country = "China"
		res.City = pointer.ToString("Hong Kong")
		res.CountryCode = "CN"
	}
	
	if strings.ToUpper(res.Country) == "TAI WAN" {
		res.Country = "China"
		res.CountryCode = "CN"
	}
	
	return
}

type QueryIP struct {
	Ip                 string   `json:"ip"`
	Network            string   `json:"network"`
	Version            string   `json:"version"`
	City               *string  `json:"city"`
	Region             *string  `json:"region"`
	RegionCode         *string  `json:"region_code"`
	Country            string   `json:"country"`
	CountryName        string   `json:"country_name"`
	CountryCode        string   `json:"country_code"`
	CountryCodeIso3    string   `json:"country_code_iso3"`
	CountryCapital     string   `json:"country_capital"`
	CountryTld         string   `json:"country_tld"`
	ContinentCode      string   `json:"continent_code"`
	InEu               *bool    `json:"in_eu"`
	Postal             string   `json:"postal"`
	Latitude           *float64 `json:"latitude"`
	Longitude          *float64 `json:"longitude"`
	Timezone           *string  `json:"timezone"`
	UtcOffset          *string  `json:"utc_offset"`
	CountryCallingCode string   `json:"country_calling_code"`
	Currency           string   `json:"currency"`
	CurrencyName       string   `json:"currency_name"`
	Languages          string   `json:"languages"`
	CountryArea        float64  `json:"country_area"`
	CountryPopulation  float64  `json:"country_population"`
	Asn                string   `json:"asn"`
	Org                string   `json:"org"`
}
