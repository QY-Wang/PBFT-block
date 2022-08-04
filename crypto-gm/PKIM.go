package crypto_gm

import (
	"github.com/DWBC-ConPeer/crypto-gm/shamir"
)

/*
 * created by yili @20181225
 * 基于门限方案将私钥分加成多个加密的片段
 * 将私钥明文分成多个明文片段，设置获取threshold（阈值）个明文片段可恢复出原文，每个明文片段使用一个账户公钥加密
 * 入参：私钥明文([]byte)，用于加密的账户([]([]byte))，阈值（可恢复私钥的最小片段数）(int)
 * 出参：加密的所有片段([]string)，error
 */
func TSS_backup(prk []byte, backuperList []([]byte), threshold int) ([]string, error) {
	// 生成threshold个明文片段
	piece, err := shamir.Split(prk, len(backuperList), threshold)
	if err != nil {
		return make([]string, 0), err
	}
	secbyte := make([][]byte, len(backuperList))
	secstr := make([]string, len(backuperList))
	for i := 0; i < len(backuperList); i++ {
		secbyte[i], err = Encrypt(ToECDSAPub(backuperList[i]), piece[i])
		if err != nil {
			return secstr, err
		}
		secstr[i] = string(secbyte[i])
	}
	return secstr, nil
}

/*
 * created by yili @20181225
 * 根据多个明文片段将数据恢复成私钥明文
 * 入参：私钥片段的明文([]([]byte))
 * 出参：私钥明文([]byte)
 */
func TSS_Recover(seglist []([]byte)) ([]byte, error) {

	// 基于多个片段（明文的）恢复私钥
	prk, err := shamir.Combine(seglist)
	if err != nil {
		return nil, err
	}
	return prk, nil
}
