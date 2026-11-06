package clientupdates

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
)

func LoadSigningKey(filename string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("更新签名密钥不可用，请检查独立密钥文件及访问权限")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.New("更新签名密钥不可读")
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("更新签名密钥格式无效")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("更新签名密钥格式无效")
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("更新签名必须使用 Ed25519 密钥")
	}
	return key, nil
}
