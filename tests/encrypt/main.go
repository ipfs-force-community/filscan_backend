package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/pkg/errors"
)

const PrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQCXS0B1UePL/6LAlS4CPyhFu/JDOf8skTH5n75/hiVY421qJUGJ
U3bteE/E4Ho2Ebil+osDa3sahgjBu1MWXoM6kfo6bV5Y50rSSpkf9Kz4+xF0DIoP
QxDwD4XYxCo12pJDrSKBcRPyAfeAZ15dNldcpTqbhPQByWNkjHQwBHfVCQIDAQAB
AoGBAIXyA8FanNkxHEBwUul+TQNgIF5QbJBig+JDAX8ZntsRjv8YuOsB0BryF31w
WAKisd2Q8Z43fCfBXuNWG3uEdZz6wIivXr5smkMDGuxqhWDSQ5V8Jhnc/aPej/WE
ywDITs+8RJS7YoSAB0mYAHUf9DiEGvyfqwXFFSRSLssXMMgBAkEAxiTbKdEc5sj5
4mvT/vJlWVEuG7v6355W9GBN8d1smAF9OZwM+tVv+ncx2y9v9nMIS2vcuwoZmkWB
SNXnK3b5gQJBAMN4YoC3C2alDkXcGSJ8e8FJgAOevxIm8a7P7dyGYkQ8L5QYFfRW
R+tVzVihwqu6HL0wOTIYHzqsJ56iqiKNz4kCQQCxXHZNVSBguI9tDIYD1KfBrnfu
XXKvzgUZ1EaQ9FnrKpIUCkpYEMueUClxgGHhIZDQKim3xs+qFwMl1kqJzoKBAkA5
S65D4GOdIMCARbWwYCC+VVcKuJt1LKkm/pfQTiu7qJChrjWxOyE1oB7i3fd78r+9
zMbXIi71OcUbQL7yBfNRAkBmSm08xHffyREh3yCT7CkKGnPagL9cRKCHY1wASfr+
xCBNpKChw24+mcnu5BIeFz/tF3sz5fkdFqbqp92jEkqd
-----END RSA PRIVATE KEY-----`

func RsaDecrypt(ciphertext []byte) ([]byte, error) {
	block, _ := pem.Decode([]byte(PrivateKey))
	if block == nil {
		return nil, errors.New("privateKey error")
	}
	privy, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptPKCS1v15(rand.Reader, privy, ciphertext)
}

func main() {
	r, err := RsaDecrypt([]byte("$2a$05$4qhoETH.MnLD2mObVRjqwOYZodpH1hsjdBHzJH2rMo.6apby8mvLO"))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(r))
}
