package MultiEncryption

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"

	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strconv"

	"encoding/base64"

	//	"github.com/DWBC-WalletPeer/common"
	//	"github.com/DWBC-WalletPeer/core/DAO"
)

// commonIV 一个不知道啥意思的变量
//var commonIV = []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}

//type PubKey struct {
//	PK []byte `json:"pk"` //公钥地址的byte数组
//}

type PubKey struct {
	PK string `json:"pk"` //公钥地址的byte数组
	//	IID common.Address `json:"iid"` //身份账户地址
}

type MultiParty struct { // 需要维护的参与方
	Owners     []string `json:"owners"`     //拥有者列表，公钥地址
	Creators     []string `json:"creators"`     // 发行方列表，公钥地址
	Supervisors []string `json:"supervisors"` // 监管方列表，公钥地址
}

//type MultiParty struct { // 需要维护的参与方
//	OwnnerList     []string `json:"ownnerList"`     //拥有者列表，公钥地址
//	IssuerList     []string `json:"issuerList"`     // 发行方列表，公钥地址
//	SupervisorList []string `json:"supervisorList"` // 监管方列表，公钥地址
//}

// 生成对称秘钥，对称秘钥长度默认为32位，keyLength为对称秘钥长度，必须为16,、24、32其中的一个
// 原理：生成一个对称的公私钥，公钥的后32位作为对称秘钥
// 返回对称密钥与相关错误
func GenerateDESKey(keyLength int) (string, error) {
	keyLength = 32 //默认为32位
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return "", errors.New("can't generate a des key")
	}
	str := strconv.Itoa((int)(privateKey.PublicKey.N.Uint64()))
	return str[1:17], nil

}

// 填充数据
func padding(src []byte, blockSize int) []byte {
	padNum := blockSize - len(src) % blockSize
	pad := bytes.Repeat([]byte{byte(padNum)}, padNum)
	return append(src, pad...)
}

// 去掉填充数据
func unpadding(src []byte) []byte {
	n := len(src)
	unPadNum := int(src[n-1])
	return src[:n-unPadNum]
}

// 使用对称秘钥加密数据
// desKey 表示加密秘钥，data表示待加密的数据
// 返回加密后的密文数据以及相应的错误
func Encryption(desKey string, data []byte) ([]byte, error) {
	block, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}

	data = padding(data, block.BlockSize())
	blockMode := cipher.NewCBCEncrypter(block, []byte(desKey))
	blockMode.CryptBlocks(data, data)

	return data, nil
}

/*func Encryption(desKey string, data []byte) ([]byte, error) {
	c, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}

	cfb := cipher.NewCFBEncrypter(c, commonIV)
	ciphertext := make([]byte, len(data))
	cfb.XORKeyStream(ciphertext, data)
	return ciphertext, nil

}*/

// 使用对称秘钥解密数据
func Decryption(desKey string, data []byte) ([]byte, error) {
	block, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}

	blockMode := cipher.NewCBCDecrypter(block, []byte(desKey))
	blockMode.CryptBlocks(data, data)
	data = unpadding(data)

	return data, nil
}

/*func Decryption(desKey string, data []byte) ([]byte, error) {
	c, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}
	cfbdec := cipher.NewCFBDecrypter(c, commonIV)
	plaintextCopy := make([]byte, len(data))
	cfbdec.XORKeyStream(plaintextCopy, data)
	return plaintextCopy, nil

}*/

// base64返回值
func Encryption2(desKey string, data []byte) (string, error) {
	block, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return "", errors.New("can't get a des key")
	}

	data = padding(data, block.BlockSize())
	blockMode := cipher.NewCBCEncrypter(block, []byte(desKey))
	blockMode.CryptBlocks(data, data)

	return base64.StdEncoding.EncodeToString(data), nil
}

/*func Encryption2(desKey string, data []byte) (string, error) {
	c, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return "", errors.New("can't get a des key")
	}

	cfb := cipher.NewCFBEncrypter(c, commonIV)
	ciphertext := make([]byte, len(data))
	cfb.XORKeyStream(ciphertext, data)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}*/

// base64格式的data
func Decryption2(desKey string, data string) ([]byte, error) {
	decodeBytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return []byte{}, err
	}

	block, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}

	blockMode := cipher.NewCBCDecrypter(block, []byte(desKey))
	blockMode.CryptBlocks(decodeBytes, decodeBytes)
	decodeBytes = unpadding(decodeBytes)

	return decodeBytes, nil
}

/*func Decryption2(desKey string, data string) ([]byte, error) {
	decodeBytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return []byte{}, err
	}
	c, err := aes.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}
	cfbdec := cipher.NewCFBDecrypter(c, commonIV)
	plaintextCopy := make([]byte, len(decodeBytes))
	cfbdec.XORKeyStream(plaintextCopy, decodeBytes)

	return plaintextCopy, nil
}*/
