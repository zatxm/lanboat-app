package profile

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"strings"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// A certificate is private node data; it is never returned in a masked node.
func ClientCertificate(n Node) (*tls.Certificate, error) {
	if n.TextOption("mutual-auth") != "true" {
		return nil, nil
	}
	input := strings.TrimSpace(n.TextOption("pkcs12"))
	var data []byte
	var err error
	if strings.HasPrefix(input, "/") || strings.HasPrefix(input, "./") || strings.HasPrefix(input, "~/") {
		if strings.HasPrefix(input, "~/") {
			home, _ := os.UserHomeDir()
			input = home + input[1:]
		}
		f, e := os.Open(input)
		if e != nil {
			return nil, errors.New("无法读取 PKCS12 文件")
		}
		defer f.Close()
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil, errors.New("PKCS12 文件需小于 1 MB")
		}
		data, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
		if len(data) > 1<<20 {
			return nil, errors.New("PKCS12 文件需小于 1 MB")
		}
	} else {
		data, err = base64.StdEncoding.DecodeString(input)
	}
	if err != nil {
		return nil, errors.New("PKCS12 需为文件路径或 Base64")
	}
	key, cert, chain, err := pkcs12.DecodeChain(data, n.TextOption("pkcs12-password"))
	if err != nil {
		return nil, errors.New("PKCS12 解密失败，请检查文件和密码")
	}
	result := &tls.Certificate{PrivateKey: key, Leaf: cert, Certificate: [][]byte{cert.Raw}}
	for _, c := range chain {
		result.Certificate = append(result.Certificate, c.Raw)
	}
	return result, nil
}
func ClientCertificatePEM(n Node) (certificate, key string, err error) {
	cert, err := ClientCertificate(n)
	if err != nil || cert == nil {
		return "", "", err
	}
	for _, der := range cert.Certificate {
		certificate += string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}
	raw, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return "", "", err
	}
	return certificate, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})), nil
}
func DecodeSHA256(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if raw, err := hex.DecodeString(strings.ReplaceAll(value, ":", "")); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if raw, err := base64.StdEncoding.DecodeString(value); err == nil && len(raw) == 32 {
		return raw, nil
	}
	return nil, errors.New("SHA256 需为 32 字节十六进制或 Base64")
}
