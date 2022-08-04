package signature

import (
	"errors"

	"github.com/DWBC-ConPeer/crypto-gm/sm3"

	"fmt"

	"github.com/DWBC-ConPeer/common"

	"github.com/DWBC-ConPeer/rlp"
)

//gm
func RlpHash(x interface{}) (h common.Hash) {
	hw := sm3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

var ErrResultLength = errors.New("return zero res")

type SchemaNoMethodErr struct {
	schemaName string
	methodName string
}

func NewSchemaNoMethodErr(schemaName string, methodName string) error {
	return &SchemaNoMethodErr{schemaName: schemaName, methodName: methodName}
}

func (m *SchemaNoMethodErr) Error() string {
	return fmt.Sprintf("%s do not have %s method", m.schemaName, m.methodName)
}

// 大文件传递数据的格式
type LFTxFormat struct {
	LFFileHashList map[string]string
}

type TransferData struct {
	Data           string      // 原数据
	LFTransferData LFTxFormat  // LF中文件的hash值
	DAOLChash      common.Hash // 资产的当前最新内容的交易hash地址
}

type DAOContract struct {
	ContractID       common.Address
	ContractVersion  string
	ContractMetaData string
}
