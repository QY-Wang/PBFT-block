// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

// Constants to match up protocol versions and messages
const (
	eth62 = 62
	eth63 = 63
)

// Official short name of the protocol used during capability negotiation.
var ProtocolName = "eth"

// Supported versions of the eth protocol (first is primary).
var ProtocolVersions = []uint{eth63, eth62}

// Number of implemented message corresponding to different protocol versions.
var ProtocolLengths = []uint64{27, 8} // must be change if you add a new message
// add each msg, the first number should add each 1

const (
	NetworkId          = 1
	ProtocolMaxMsgSize = 100 * 1024 * 1024 // Maximum cap on the size of a protocol message
)

// eth protocol message codes
const (
	// Protocol messages belonging to eth/62
	StatusMsg          = 0x00
	NewBlockHashesMsg  = 0x01
	TxMsg              = 0x02
	GetBlockHeadersMsg = 0x03
	BlockHeadersMsg    = 0x04
	GetBlockBodiesMsg  = 0x05
	BlockBodiesMsg     = 0x06
	NewBlockMsg        = 0x07
	// pbft message
	//RequsetMsg    = 0x08
	//PrePrepareMsg = 0x09
	//PrepareMsg    = 0x0a
	//CommitMsg     = 0x0b
	//CheckpointMsg = 0x0c
	//NewViewMsg    = 0x0d
	//ViewChangeMsg = 0x0f

	//改：共识消息和checkpoint消息
	//AccountStatusRequestMsg = 0x08
	//AccountStatusReplyMsg   = 0x09
	ConMsg    = 0x08
	Checkpoin = 0x09
	Zhuanfa   = 0x10

	//	NodetypeRequestMsg = 0x12
	//	NodeTypeReplyMsg   = 0x13

	AskTransactionByHashMsg  = 0x0a // 12 与 13 这两条消息替换掉原来的nodetype消息
	RelyTransactionByHashMsg = 0x0b

	// Protocol messages belonging to eth/63
	GetNodeDataMsg = 0x0c
	NodeDataMsg    = 0x0d
	GetReceiptsMsg = 0x0e
	ReceiptsMsg    = 0x0f

	//AskLFDataMsg  = 0x10  改：转发消息使用了
	RelyLFDataMsg = 0x11

	TestSendMsg   = 0x12 // consenesus msg
	TestReciveMsg = 0x13
	GetTxMsg      = 0x14
	TxsMsg        = 0x15
)

type errCode int

const (
	ErrMsgTooLarge = iota
	ErrDecode
	ErrInvalidMsgCode
	ErrProtocolVersionMismatch
	ErrNetworkIdMismatch
	ErrGenesisBlockMismatch
	ErrNoStatusMsg
	ErrExtraStatusMsg
	ErrSuspendedPeer
	ErrIllegalSig
	ErrInvalidVRS
	ErrHighBlock
	ErrGetVRS
	ErrDeCmpress
	ErrInvalidBlock
)

// func (e errCode) String() string {
// 	return errorToString[int(e)]
// }

// XXX change once legacy code is out
// var errorToString = map[int]string{
// 	ErrMsgTooLarge:             "Message too long",
// 	ErrDecode:                  "Invalid message",
// 	ErrInvalidMsgCode:          "Invalid message code",
// 	ErrProtocolVersionMismatch: "Protocol version mismatch",
// 	ErrNetworkIdMismatch:       "NetworkId mismatch",
// 	ErrGenesisBlockMismatch:    "Genesis block mismatch",
// 	ErrNoStatusMsg:             "No status message",
// 	ErrExtraStatusMsg:          "Extra status message",
// 	ErrSuspendedPeer:           "Suspended peer",
// }

// type txPool interface {
// 	// AddBatch should add the given transactions to the pool.
// 	AddBatch([]*TXDM.Transaction)
// 	Add(tx *TXDM.Transaction) error
// 	Add_TM(tx *TXDM.Transaction) error
// 	TxPoolManager() *core.TxPoolManager
// 	// Pending should return pending transactions.
// 	// The slice should be modifiable by the caller.
// 	Pending() map[common.Address]TXDM.Transactions

// 	GetAccountStatus(addr common.Address) (*big.Int, uint64, common.Hash)
// 	SetAccountStatus(addr common.Address, balance *big.Int, nonce uint64, lt common.Hash)

// 	Get(hash common.Hash) *TXDM.Transaction
// }

// statusData is the network packet for the status message.
// type statusData struct {
// 	ProtocolVersion uint32
// 	NetworkId       uint32
// 	TD              *big.Int
// 	CurrentBlock    common.Hash
// 	GenesisBlock    common.Hash
// }

// // newBlockHashesData is the network packet for the block announcements.
// type newBlockHashesData []struct {
// 	Hash   common.Hash // Hash of one particular block being announced
// 	Number uint64      // Number of one particular block being announced
// }

// getBlockHeadersData represents a block header query.
// type getBlockHeadersData struct {
// 	Origin  hashOrNumber // Block from which to retrieve headers
// 	Amount  uint64       // Maximum number of headers to retrieve
// 	Skip    uint64       // Blocks to skip between consecutive headers
// 	Reverse bool         // Query direction (false = rising towards latest, true = falling towards genesis)
// }

// hashOrNumber is a combined field for specifying an origin block.
// type hashOrNumber struct {
// 	Hash   common.Hash // Block hash from which to retrieve headers (excludes Number)
// 	Number uint64      // Block hash from which to retrieve headers (excludes Hash)
// }

// EncodeRLP is a specialized encoder for hashOrNumber to encode only one of the
// two contained union fields.
// func (hn *hashOrNumber) EncodeRLP(w io.Writer) error {
// 	if hn.Hash == (common.Hash{}) {
// 		return rlp.Encode(w, hn.Number)
// 	}
// 	if hn.Number != 0 {
// 		return fmt.Errorf("both origin hash (%x) and number (%d) provided", hn.Hash, hn.Number)
// 	}
// 	return rlp.Encode(w, hn.Hash)
// }

// // DecodeRLP is a specialized decoder for hashOrNumber to decode the contents
// // into either a block hash or a block number.
// func (hn *hashOrNumber) DecodeRLP(s *rlp.Stream) error {
// 	_, size, _ := s.Kind()
// 	origin, err := s.Raw()
// 	if err == nil {
// 		switch {
// 		case size == 32:
// 			err = rlp.DecodeBytes(origin, &hn.Hash)
// 		case size <= 8:
// 			err = rlp.DecodeBytes(origin, &hn.Number)
// 		default:
// 			err = fmt.Errorf("invalid input size %d for origin", size)
// 		}
// 	}
// 	return err
// }

// newBlockData is the network packet for the block propagation message. newBlockData 是块传播消息的网络数据包
// type newBlockData struct {
// 	Block *BlockDM.Block
// 	TD    *big.Int
// }

// type newBlockInfo struct {
// 	//VRS 	[]byte
// 	BlockWithVRS *CommunicationInfo
// 	TD           *big.Int
// }

// // blockBody represents the data content of a single block.
// type blockBody struct {
// 	Transactions []*TXDM.Transaction // Transactions contained within a block
// 	Uncles       []*HeaderDM.Header  // Uncles contained within a block
// }

// // blockBodiesData is the network packet for block content distribution.
// type blockBodiesData []*blockBody
