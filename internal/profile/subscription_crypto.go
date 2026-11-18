package profile

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"errors"
	"strings"
	"unicode/utf8"
)

// Karing's subscription-encryption=true format: Base64(IV || AES-128-CBC-PKCS7).
// MD5 is required for wire compatibility, not used to store or verify passwords.
func DecryptSubscription(content, password string) (string, error) {
	if len(content) > MaxSubscription {
		return "", errors.New("订阅内容不能超过 32 MiB")
	}
	if password == "" {
		return "", errors.New("此订阅已加密，请填写解密密码")
	}
	fail := errors.New("订阅解密失败，请检查密码或加密内容")
	raw, err := DecodeBase64(strings.Join(strings.Fields(content), ""))
	if err != nil || len(raw) < 2*aes.BlockSize || len(raw)%aes.BlockSize != 0 {
		return "", fail
	}
	key := md5.Sum([]byte(password))
	block, _ := aes.NewCipher(key[:])
	plain := raw[aes.BlockSize:]
	cipher.NewCBCDecrypter(block, raw[:aes.BlockSize]).CryptBlocks(plain, plain)
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
		return "", fail
	}
	for _, b := range plain[len(plain)-padding:] {
		if int(b) != padding {
			return "", fail
		}
	}
	plain = plain[:len(plain)-padding]
	if len(plain) == 0 || !utf8.Valid(plain) {
		return "", fail
	}
	return string(plain), nil
}
