package evm_signature

import "time"

type EventSignatureList struct {
	Count    int64             `json:"count"`
	Next     string            `json:"next"`
	Previous string            `json:"previous"`
	Results  []*EventSignature `json:"results"`
}

type EventSignature struct {
	Id             int64     `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	TextSignature  string    `json:"text_signature"`
	HexSignature   string    `json:"hex_signature"`
	BytesSignature string    `json:"bytes_signature"`
}
