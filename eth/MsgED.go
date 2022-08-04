package eth

import (
	"bytes"
	"encoding/base64"
	"errors"

	crypto "github.com/DWBC-ConPeer/crypto-gm"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/signature"

	"github.com/DWBC-ConPeer/common"

	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
)

// 返回通信消息内容的hash值
func (ci *CommunicationInfo) SigHash() common.Hash {
	return signature.RlpHash([]interface{}{ //不知道为啥换个名字就行了
		ci.PayLoad,
	})
}

/*
 * Sign 私钥签名
 * 入参data，通信消息的内容
 * 入参NodePrk，签名节点私钥
 * 返回签名后的通信消息
 */
func Sign(data []byte, NodePrk *sm2.PrivateKey) (*CommunicationInfo, error) {

	cpy := CommunicationInfo{PayLoad: data}

	sdata, err := crypto.Sign(cpy.SigHash().Bytes(), NodePrk) // 使用私钥对data的hash值签名
	if err != nil {
		glog.V(logger.Info).Infoln("sign err:", err)
	}

	v, r, s, err := sm2.SignDataToSignDigit(sdata)
	if err != nil {
		return &cpy, err
	}
	cpy.Sig.R = r
	cpy.Sig.S = s
	cpy.Sig.V = v
	return &cpy, nil
}

var ErrInvalidSig = errors.New("invalid v, r, s values")

/*
 * ValidateSig验证消息的签名是否正确
 * 入参nodepk，签名节点的公钥
 * 入参cinfo，签名后的通信消息
 * 返回验签结果
 */
func ValidateSig(nodepk []byte, cinfo CommunicationInfo, homestead bool) (bool, error) {

	//// 验证签名数据格式是否正确
	//if !crypto.ValidateSignatureValues(cinfo.Sig.V, cinfo.Sig.R,
	//	cinfo.Sig.S, homestead) {
	//	return false, ErrInvalidSig
	//}
	//
	//r, s := cinfo.Sig.R.Bytes(), cinfo.Sig.S.Bytes()
	//sig := make([]byte, 65)
	//copy(sig[32-len(r):32], r)
	//copy(sig[64-len(s):64], s)
	//sig[64] = cinfo.Sig.V - 27
	//
	////计算通信消息内容hash
	//hash := cinfo.SigHash()
	//
	////调用Ecrecover（椭圆曲线签名恢复）来检索签名者的公钥
	//pub, err := crypto.Ecrecover(hash[:], sig)
	//
	//if err != nil {
	//	glog.V(logger.Error).Infof("Could not get pubkey from signature: ", err)
	//	return false, err
	//}
	//if len(pub) == 0 || pub[0] != 4 {
	//	return false, errors.New("invalid public key")
	//}
	//
	//// 校验计算出来的公钥与传入的公钥是否相等
	//if bytes.Equal(pub, nodepk) {
	//	return true, nil
	//}
	//if glog.V(logger.Debug){
	//	glog.Infof("byte sigcompute key %v",pub)
	//	glog.Infof("byte node key %v",nodepk)
	//	glog.Infof("base64 sigcompute key %v",base64.StdEncoding.EncodeToString(pub))
	//	glog.Infof("base64 node key %v",base64.StdEncoding.EncodeToString(nodepk))
	//}
	//return false, errors.New("nodepk and pk is not equal")
	hash := cinfo.SigHash()

	sig, err := sm2.SignDigitToSignData(cinfo.Sig.V, cinfo.Sig.R, cinfo.Sig.S)
	if err != nil {
		return false, errors.New("invalid sign")
	}
	pub, err := crypto.Ecrecover(hash[:], sig)
	if err != nil {
		glog.V(logger.Error).Infof("Could not get pubkey from signature: ", err)
		return false, err
	}
	b, _ := base64.StdEncoding.DecodeString(string(nodepk)) //自己加的
	// 校验签名中的公钥与传入的公钥是否相等
	if bytes.Equal(pub, b) {
		return true, nil
	}
	if glog.V(logger.Debug) {
		glog.Infof("byte sigcompute key %v", pub)
		glog.Infof("byte node key %v", nodepk)
		glog.Infof("base64 sigcompute key %v", base64.StdEncoding.EncodeToString(pub))
		glog.Infof("base64 node key %v", base64.StdEncoding.EncodeToString(nodepk))
	}
	return false, errors.New("nodepk and pk is not equal")
}
