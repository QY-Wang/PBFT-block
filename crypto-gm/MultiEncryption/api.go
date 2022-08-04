package MultiEncryption

import (
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/crypto-gm/sm4"
	r "math/rand"
	"strconv"
	"time"
)

type MultiParty struct { // 需要维护的参与方
	Owners     []string `json:"owners"`     //拥有者列表，公钥地址
	Creators     []string `json:"creators"`     // 发行方列表，公钥地址
	Supervisors []string `json:"supervisors"` // 监管方列表，公钥地址
}

func GenerateDESKey(keyLength int) (string, error) {
	keyLength = 32 //默认为32位
	privateKey, err := sm2.GenerateKey()
	if err != nil {
		return "", errors.New("can't generate a des key")
	}
	str := strconv.Itoa((int)(privateKey.PublicKey.X.Int64()))
	if len(str) < 17 {
		num := 17 - len(str)
		temp := Krand(num, 0) // 0代表生成数字   1小写字母   2大写字母  3数字大小写字母
		str = str + temp
	}
	deskey := str[1:17]

	return deskey, nil
}

// 生成指定长度的随机数
func Krand(size int, kind int) string {
	ikind, kinds, result := kind, [][]int{[]int{10, 48}, []int{26, 97}, []int{26, 65}}, make([]byte, size)
	is_all := kind > 2 || kind < 0
	r.Seed(time.Now().UnixNano())
	for i := 0; i < size; i++ {
		if is_all { // random ikind
			ikind = r.Intn(3)
		}
		scope, base := kinds[ikind][0], kinds[ikind][1]
		result[i] = uint8(base + r.Intn(scope))
	}
	return string(result)
}

//func padding(src []byte, blockSize int) []byte {
//	padNum := blockSize - len(src)%blockSize
//	pad := bytes.Repeat([]byte{byte(padNum)}, padNum)
//	return append(src, pad...)
//}


//func unpadding(src []byte) []byte {
//	n := len(src)
//	unPadNum := int(src[n-1])
//	return src[:n-unPadNum]
//}

// 填充数据
func padding(in []byte, blockSize int) []byte {
	padding := blockSize - (len(in) % blockSize)
	for i := 0; i < padding; i++ {
		in = append(in, byte(padding))
	}
	return in
}

// 去掉填充数据
func unpadding(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}

	padding := in[len(in)-1]
	if int(padding) > len(in) || padding > sm4.BlockSize {
		return nil
	} else if padding == 0 {
		return nil
	}

	for i := len(in) - 1; i > len(in)-int(padding)-1; i-- {
		if in[i] != padding {
			return nil
		}
	}
	return in[:len(in)-int(padding)]
}

func Encryption(desKey string, data []byte) ([]byte, error) {
	block, err := sm4.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}

	data = padding(data, block.BlockSize())
	blockMode := cipher.NewCBCEncrypter(block, []byte(desKey))
	blockMode.CryptBlocks(data, data)

	return data, nil
}

func Decryption(desKey string, data []byte) ([]byte, error) {
	block, err := sm4.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}

	blockMode := cipher.NewCBCDecrypter(block, []byte(desKey))
	blockMode.CryptBlocks(data, data)
	data = unpadding(data)

	return data, nil
}

//func Encryption(desKey string, data []byte) ([]byte, error) {
//	c, err := sm4.NewCipher([]byte(desKey))
//	if err != nil {
//		fmt.Println("sm4.NewCipher err: ", err)
//		return nil, err
//	}
//
//	data = padding(data, c.BlockSize())
//
//	d0 := make([]byte, 0)
//
//	n := len(data) / c.BlockSize()
//	for i := 0; i < n; i++ {
//		temp := make([]byte, c.BlockSize())
//		c.Encrypt(temp, data[i*c.BlockSize():i*c.BlockSize()+c.BlockSize()])
//		for _, v := range temp {
//			d0 = append(d0, v)
//		}
//	}
//	return d0, nil
//}
//
//func Decryption(desKey string, data []byte) ([]byte, error) {
//	c, err := sm4.NewCipher([]byte(desKey))
//	if err != nil {
//		fmt.Println("sm4.NewCipher err: ", err)
//		return nil, err
//	}
//
//	d1 := make([]byte, 0)
//
//	n := len(data) / c.BlockSize()
//
//	for i := 0; i < n; i++ {
//		temp := make([]byte, c.BlockSize())
//		c.Decrypt(temp, data[i*c.BlockSize():i*c.BlockSize()+c.BlockSize()])
//		if i == (n-1) {
//			temp = unpadding(temp)
//		}
//		for _, v := range temp {
//			d1 = append(d1, v)
//		}
//	}
//
//	return d1, nil
//}

func Encryption2(desKey string, data []byte) (string, error) {
	block, err := sm4.NewCipher([]byte(desKey))
	if err != nil {
		return "", errors.New("can't get a des key")
	}

	data = padding(data, block.BlockSize())
	blockMode := cipher.NewCBCEncrypter(block, []byte(desKey))
	blockMode.CryptBlocks(data, data)

	return base64.StdEncoding.EncodeToString(data), nil
}

func Decryption2(desKey string, data string) ([]byte, error) {
	decodeBytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return []byte{}, err
	}

	block, err := sm4.NewCipher([]byte(desKey))
	if err != nil {
		return nil, errors.New("can't get a des key")
	}

	blockMode := cipher.NewCBCDecrypter(block, []byte(desKey))
	blockMode.CryptBlocks(decodeBytes, decodeBytes)
	decodeBytes = unpadding(decodeBytes)

	return decodeBytes, nil
}
