package crypto_gm

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/DWBC-ConPeer/common"
	"github.com/DWBC-ConPeer/crypto-gm/MultiEncryption"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"io/ioutil"
	"os"
	"testing"
)

func TestKeccak256(t *testing.T) {
	msg := []byte("abc")
	res := Keccak256(msg)
	fmt.Println(len(res))

	res2 := Keccak256Hash(msg)
	fmt.Println(res2.Hex())

	res3 := Sha3(msg)
	fmt.Println(len(res3))

	res4 := Sha3Hash(msg)
	fmt.Println(res4.Hex())

	res5 := CreateAddress(common.HexToAddress("0x3e899297129204eb9cc94883a052fc65b56a6eee"), 7)
	fmt.Println(res5.Hex())
}

func TestSha256AndRipemd160(t *testing.T) {
	msg := []byte("abcjfklnfldnfln")
	res := Sha256(msg)
	fmt.Println(res)
	fmt.Println(hex.EncodeToString(res))
	fmt.Println(len(res))

	//res2 := Ripemd160(msg)
	//fmt.Println(len(res2))
}

func TestFromAndTo(t *testing.T) {
	prk, err := GenerateKey()
	if err != nil {
		fmt.Println("GenerateKey err: ", err)
		return
	}
	fmt.Println("prk: ", prk)

	prkbytes := FromECDSA(prk)
	fmt.Println("prkbytes: ", prkbytes)

	prk = ToECDSA(prkbytes)
	fmt.Println("prk: ", prk)

	fmt.Println("---------------------------")

	pub := prk.PublicKey
	fmt.Println("pub: ", pub)

	pubbytes := FromECDSAPub(&pub)
	fmt.Println("pubbytes: ", pubbytes)

	pub2 := ToECDSAPub(pubbytes)
	fmt.Println("pub: ", pub2)
}

var testAddrHex = "970e8128ab834e8eac17ab8e3812f010678cf791"
var testPrivHex = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"

func TestHexToECDSA(t *testing.T) {
	prk, err := HexToECDSA(testPrivHex)
	if err != nil {
		fmt.Println("HexToECDSA err: ", err)
		return
	}
	fmt.Println("prk: ", prk)

	prk, err = LoadECDSAByPrk(testPrivHex)
	if err != nil {
		fmt.Println("LoadECDSAByPrk err: ", err)
		return
	}
	fmt.Println("prk: ", prk)
}

func TestLoadECDSA(t *testing.T) {
	fileName0 := "test_key0"
	ioutil.WriteFile(fileName0, []byte(testPrivHex), 0600)
	defer os.Remove(fileName0)

	prkey0, err := LoadECDSA(fileName0)
	if err != nil {
		fmt.Println("LoadECDSA err: ", err)
		return
	}
	fmt.Println("prkey: ", prkey0)

	fileName1 := "test_key1"
	err = SaveECDSA(fileName1, prkey0)
	if err != nil {
		fmt.Println("SaveECDSA err: ", err)
		return
	}
}

func TestSign(t *testing.T) {
	msg := []byte("uijkmnhgtfedsdftyuioplmjhgfdsazx")
	prk, err := GenerateKey()
	if err != nil {
		fmt.Println("GenerateKey err: ", err)
		return
	}
	fmt.Println("prk: ", len(FromECDSA(prk)))
	fmt.Println("prk.PublicKey: ", FromECDSAPub(&prk.PublicKey))
	fmt.Println("prk.PublicKey-base64: ", base64.StdEncoding.EncodeToString(FromECDSAPub(&prk.PublicKey)))

	sig, err := Sign(msg, prk)
	if err != nil {
		fmt.Println("Sign err: ", err)
		return
	}
	fmt.Println("--------: ", len(sig))

	pub, r, s, err := sm2.SignDataToSignDigit(sig)
	if err != nil {
		fmt.Println("sm2.SignDataToSignDigit err: ", err)
		return
	}
	fmt.Println("pub: ", pub)
	fmt.Println("pub-base64: ", base64.StdEncoding.EncodeToString(pub))
	fmt.Println("r: ", r)
	fmt.Println("s: ", s)

	flag := Verify(msg, sig)
	fmt.Println("flag: ", flag)

	//flag = ValidateSignatureValues(pub, msg, sig)	// 验证失败   此函数不再提供
	//flag = sm2.VerifyBytes(prk.PublicKey.X.Bytes(), prk.PublicKey.Y.Bytes(), msg, nil, r.Bytes(), s.Bytes())
	//fmt.Println("flag: ", flag)

	pubbytes, err := Ecrecover(msg, sig)
	if err != nil {
		fmt.Println("Ecrecover err: ", err)
		return
	}
	fmt.Println("pubbytes: ", pubbytes)
}

func TestED(t *testing.T) {
	prk, err := GenerateKey()
	if err != nil {
		fmt.Println("GenerateKey err: ", err)
		return
	}

	msg := []byte("abckop09dkfjdlnfdhjlnlkdshfsnflsafnldfndkn,jerjhelknlgnkrlng")

	encData, err := Encrypt(&prk.PublicKey, msg)
	if err != nil {
		fmt.Println("Encrypt err: ", err)
		return
	}
	fmt.Println("encData: ", encData)

	oriData, err := Decrypt(prk, encData)
	if err != nil {
		fmt.Println("Decrypt err: ", err)
		return
	}
	fmt.Println("oriData: ", string(oriData))
}

func TestPubkeyToAddress(t *testing.T) {
	prk, err := GenerateKey()
	if err != nil {
		fmt.Println("GenerateKey err: ", err)
		return
	}

	addr := PubkeyToAddress(prk.PublicKey)
	fmt.Println("addr: ", addr)
	fmt.Println("addr: ", addr.Hex())
}

// -----------------------------------------------

func TestMulti1(t *testing.T) {
	deskeyStr, err := MultiEncryption.GenerateDESKey(32)
	if err != nil {
		fmt.Println("MultiEncryption.GenerateDESKey err: ", err)
		return
	}
	fmt.Println("deskey: ", deskeyStr)

	msg := []byte("abu")
	fmt.Println("msg: ", string(msg))
	encryData, err := MultiEncryption.Encryption(deskeyStr, msg)
	if err != nil {
		fmt.Println("MultiEncryption.Encryption err: ", err)
		return
	}
	fmt.Println("encryData: ", encryData)

	decData, err := MultiEncryption.Decryption(deskeyStr, encryData)
	if err != nil {
		fmt.Println("MultiEncryption.Decryption err: ", err)
		return
	}
	fmt.Println("decData: ", string(decData))
}

func TestMulti2(t *testing.T) {
	deskeyStr, err := MultiEncryption.GenerateDESKey(32)
	if err != nil {
		fmt.Println("MultiEncryption.GenerateDESKey err: ", err)
		return
	}
	fmt.Println("deskey: ", deskeyStr)

	msg := []byte("abukfdjfdndlnfkdlnfldskfjdklfjkdlfjdsl")
	fmt.Println("msg: ", string(msg))
	encryData, err := MultiEncryption.Encryption2(deskeyStr, msg)
	if err != nil {
		fmt.Println("MultiEncryption.Encryption err: ", err)
		return
	}
	fmt.Println("encryData: ", encryData)

	decData, err := MultiEncryption.Decryption2(deskeyStr, encryData)
	if err != nil {
		fmt.Println("MultiEncryption.Decryption err: ", err)
		return
	}
	fmt.Println("decData: ", string(decData))
}

func TestPKIM(t *testing.T) {
	prk,_ := GenerateKey()
	fmt.Println(prk)
	prkbyte := FromECDSA(prk)
	fmt.Println(prkbyte)

	prk0,_ := GenerateKey()
	prk1,_ := GenerateKey()
	prk2,_ := GenerateKey()

	pub0 := FromECDSAPub(&prk0.PublicKey)
	pub1 := FromECDSAPub(&prk1.PublicKey)
	pub2 := FromECDSAPub(&prk2.PublicKey)


	pubs := make([]([]byte), 3)
	pubs[0] = pub0
	pubs[1] = pub1
	pubs[2] = pub2

	res, err := TSS_backup(prkbyte, pubs, 3)
	if err != nil {
		fmt.Println("TSS_backup err: ", err)
		return
	}
	//fmt.Print(res)

	d1, err := Decrypt(prk0, []byte(res[0]))
	if err != nil {
		fmt.Println("Decrypt1 err: ", err)
		return
	}

	d2, err := Decrypt(prk1, []byte(res[1]))
	if err != nil {
		fmt.Println("Decrypt2 err: ", err)
		return
	}

	d3, err := Decrypt(prk2, []byte(res[2]))
	if err != nil {
		fmt.Println("Decrypt3 err: ", err)
		return
	}

	datas := make([]([]byte), 3)
	datas[0] = d1
	datas[1] = d2
	datas[2] = d3

	prks, err := TSS_Recover(datas)
	if err != nil {
		fmt.Println("TSS_Recover err: ", err)
		return
	}
	fmt.Println(ToECDSA(prks))
	fmt.Println(prks)
}

//func TestPKIM2(t *testing.T) {
//	prk := "818a28855befbb05e705f2a8e6eeea017ee5b4d4d7c47cee9db30b2d9211cbfc"
//
//	pub1 := "BHDIBS/ZFwW8UFX0weVrjaQHiJ4270TAJyW8PlMWz9B8+ODUCbZVgMRiPUJvfZXey/Dmnm2+OcMWFDKAoz8DwNM="
//	pub1Bytes, err := base64.StdEncoding.DecodeString(pub1)
//	if err != nil {
//		return
//	}
//
//	pub2 := "BP3uf9tQj7oaykn+ltvHBh+CbS03w6v4tU+eTGvAJEjXr6xUrCbVWDcJc+DEgVlnk52bZGMD11MIMRVfCTNRjF4="
//	pub2Bytes, err := base64.StdEncoding.DecodeString(pub2)
//	if err != nil {
//		return
//	}
//
//	pub3 := "BEDEUmE4qDxxaX3DM8Z6waQblf38O+NNhl39CUgUiTN+R2yhnRn8kKa30PH237PYE3WuivdzzX9bOAJfLLJKNlQ="
//	pub3Bytes, err := base64.StdEncoding.DecodeString(pub3)
//	if err != nil {
//		return
//	}
//
//	pubs := make([]([]byte), 3)
//	pubs[0] = pub1Bytes
//	pubs[1] = pub2Bytes
//	pubs[2] = pub3Bytes
//
//	fmt.Println(pubs)
//
//	res, err := TSS_backup([]byte(prk), pubs, 3)
//	if err != nil {
//		fmt.Println("TSS_backup err: ", err)
//		return
//	}
//	fmt.Print("res: ", res)
//}
