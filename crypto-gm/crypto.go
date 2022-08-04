package crypto_gm

import (
	"crypto/elliptic"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/DWBC-ConPeer/common"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/crypto-gm/sm3"
	"github.com/DWBC-ConPeer/rlp"
	"golang.org/x/crypto/ripemd160"
)

func Keccak256(data ...[]byte) []byte {
	d := sm3.New()
	for _, b := range data {
		d.Write(b)
	}
	return d.Sum(nil)
}

func Keccak256Hash(data ...[]byte) (h common.Hash) {
	d := sm3.New()
	for _, b := range data {
		d.Write(b)
	}
	d.Sum(h[:0])
	return h
}

func Sha3(data ...[]byte) []byte          { return Keccak256(data...) }
func Sha3Hash(data ...[]byte) common.Hash { return Keccak256Hash(data...) }

// Creates an ethereum address given the bytes and the nonce
func CreateAddress(b common.Address, nonce uint64) common.Address {
	data, _ := rlp.EncodeToBytes([]interface{}{b, nonce})
	return common.BytesToAddress(Keccak256(data)[12:])
}

func Sha256(data []byte) (digest []byte) {
	return sm3.Sum256(data)
}

func Ripemd160(data []byte) []byte {
	ripemd := ripemd160.New()
	ripemd.Write(data)

	return ripemd.Sum(nil)
}

func Ecrecover(hash, sig []byte) ([]byte, error) {
	// 需要先验证
	flag := Verify(hash, sig)
	if !flag {
		return nil, errors.New("verify failed")
	}

	pub, _, _, err := sm2.SignDataToSignDigit(sig)
	if err != nil {
		fmt.Println("sm2.SignDataToSignDigit err: ", err)
		return nil, err
	}

	return pub, nil
}

func ToECDSA(prv []byte) *sm2.PrivateKey {
	if len(prv) == 0 {
		return nil
	}

	priv := new(sm2.PrivateKey)
	priv.PublicKey.Curve = sm2.P256Sm2()
	priv.D = common.BigD(prv)
	priv.PublicKey.X, priv.PublicKey.Y = sm2.P256Sm2().ScalarBaseMult(prv)
	return priv
}

func FromECDSA(prv *sm2.PrivateKey) []byte {
	if prv == nil {
		return nil
	}
	return prv.D.Bytes()
}

func ToECDSAPub(pub []byte) *sm2.PublicKey {
	if len(pub) == 0 {
		return nil
	}
	x, y := elliptic.Unmarshal(sm2.P256Sm2(), pub)
	return &sm2.PublicKey{Curve: sm2.P256Sm2(), X: x, Y: y}
}

func FromECDSAPub(pub *sm2.PublicKey) []byte {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return nil
	}
	return elliptic.Marshal(sm2.P256Sm2(), pub.X, pub.Y)
}

func HexToECDSA(hexkey string) (*sm2.PrivateKey, error) {
	b, err := hex.DecodeString(hexkey)
	if err != nil {
		return nil, errors.New("invalid hex string")
	}
	if len(b) != 32 {
		return nil, errors.New("invalid length, need 256 bits")
	}
	return ToECDSA(b), nil
}

func LoadECDSA(file string) (*sm2.PrivateKey, error) {
	buf := make([]byte, 64)
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	if _, err := io.ReadFull(fd, buf); err != nil {
		return nil, err
	}

	key, err := hex.DecodeString(string(buf))
	if err != nil {
		return nil, err
	}

	return ToECDSA(key), nil
}

func LoadECDSAByPrk(prk string) (*sm2.PrivateKey, error) {

	key, err := hex.DecodeString(prk)
	if err != nil {
		return nil, err
	}

	return ToECDSA(key), nil
}

func SaveECDSA(file string, key *sm2.PrivateKey) error {
	k := hex.EncodeToString(FromECDSA(key))
	return ioutil.WriteFile(file, []byte(k), 0600)
}

func GenerateKey() (*sm2.PrivateKey, error) {
	return sm2.GenerateKey()
}

//type sm2Signature struct {
//	R, S *big.Int
//}
////func ValidateSignatureValues(v byte, r, s *big.Int, homestead bool) bool {
//func ValidateSignatureValues(publicKey, hash, signature []byte) bool {
//	_, r, s, err := sm2.SignDataToSignDigit(signature)
//	if err != nil {
//		fmt.Println("sm2.SignDataToSignDigit err: ", err)
//		return false
//	}
//
//	sigs, err := signDigitToSignData(r, s)
//	if err != nil {
//		fmt.Println("signDigitToSignData err: ", err)
//		return false
//	}
//
//	return sm2.Sm2VerifyBytes(publicKey, hash, sigs)
//}
//func signDigitToSignData(r, s *big.Int) ([]byte, error) {
//	return asn1.Marshal(sm2Signature{r, s})
//}

// 未实现
//func SigToPub(hash, sig []byte) (*sm2.PublicKey, error) {
//	return nil, nil
//}

func Sign(hash []byte, prv *sm2.PrivateKey) (sig []byte, err error) {
	r, s, err := sm2.Sign(prv, hash)
	if err != nil {
		fmt.Println("Sign err: ", err)
		return nil, err
	}
	// 获取公钥
	pub := prv.PublicKey

	sig, err = sm2.SignDigitToSignData(FromECDSAPub(&pub), r, s)
	if err != nil {
		fmt.Println("SignDigitToSignData err: ", err)
		return nil, err
	}
	return sig, nil
}

// 验签
func Verify(hash, sig []byte) bool {
	pub, r, s, err := sm2.SignDataToSignDigit(sig)
	if err != nil {
		fmt.Println("sm2.SignDataToSignDigit err: ", err)
		return false
	}

	return sm2.Verify(ToECDSAPub(pub), hash, r, s)
}

func Encrypt(pub *sm2.PublicKey, message []byte) ([]byte, error) {
	return sm2.Encrypt(pub, message)
}

func Decrypt(prv *sm2.PrivateKey, ct []byte) ([]byte, error) {
	if ct == nil {
		return nil, errors.New("input data illegal")
	}
	return sm2.Decrypt(prv, ct)
}

func PubkeyToAddress(p sm2.PublicKey) common.Address {
	pubBytes := FromECDSAPub(&p)
	return common.BytesToAddress(Keccak256(pubBytes[1:])[12:])
}
