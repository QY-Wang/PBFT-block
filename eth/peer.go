// Copyright 2015 The go-ethereum Authors
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

import (
	"errors"
	"fmt"
	// "github.com/DWBC-ConPeer/core/VRS"
	// "github.com/DWBC-ConPeer/core/dencom"
	// "github.com/DWBC-ConPeer/core/types/HeaderWithVRS"
	"math/big"
	"sync"
	"time"

	"github.com/DWBC-ConPeer/common"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/rlp"

	// "github.com/DWBC-ConPeer/core/types/BlockDM"
	// "github.com/DWBC-ConPeer/core/types/TXDM"

	"github.com/DWBC-ConPeer/p2p"
)

type CommunicationInfo struct {
	PayLoad []byte
	Sig     Signature
	VRS     []byte
}

type Signature struct {
	V    []byte   // signature 公钥
	R, S *big.Int // signature
}

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

const (
	maxKnownTxs      = 327680 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks   = 1024   // Maximum block hashes to keep in the known list (prevent DOS)
	handshakeTimeout = 5 * time.Second
)

// PeerInfo represents a short summary of the Ethereum sub-protocol metadata known
// about a connected peer.
type PeerInfo struct {
	Version    int      `json:"version"`    // Ethereum protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the peer's blockchain
	Head       string   `json:"head"`       // SHA3 hash of the peer's best owned block
}

type peer struct {
	id string

	*p2p.Peer
	rw p2p.MsgReadWriter

	version  int         // Protocol version negotiated
	forkDrop *time.Timer // Timed connection dropper if forks aren't validated in time

	head common.Hash
	td   *big.Int
	lock sync.RWMutex

	// knownTxs    *set.Set // Set of transaction hashes known to be known by this peer
	// knownBlocks *set.Set // Set of block hashes known to be known by this peer
	//knownRequests    *set.Set // add a core.Request knownList to suit pbft
	//knownPrePrepares *set.Set // add a core.PrePrepare knownList
	//knownPrepares    *set.Set // add a core.Prepare knownList
	//knownCommits     *set.Set // add a core.PrePrepare knownList
	//knownCheckpoints *set.Set // add a core.PrePrepare knownList
	//knownNewViews    *set.Set // add a core.PrePrepare knownList
	//knownViewChanges *set.Set // add a core.PrePrepare knownList
	NodeType uint64

	// knownAccountStatusRequests *set.Set
	// knownAccountStatusReplys   *set.Set

	// knownConMsg  *set.Set
	// knownPbftMsg *set.Set

	blockheight uint64 // 区块高度 created by mxp @20180918
	// TODO 单独的锁，与head的锁区分开，以免造成死锁
	NodePK []byte //节点公钥

}

func (p *peer) GetPeerID() string {
	return p.id
}
func newPeer(version int, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	id := p.ID()

	return &peer{
		Peer:    p,
		rw:      rw,
		version: version,
		id:      fmt.Sprintf("%x", id[:]),
		// knownTxs:    set.New(),
		// knownBlocks: set.New(),
		//knownRequests:              set.New(),
		//knownPrePrepares:           set.New(),
		//knownPrepares:              set.New(),
		//knownCommits:               set.New(),
		//knownCheckpoints:           set.New(),
		//knownNewViews:              set.New(),
		//knownViewChanges:           set.New(),
		// knownAccountStatusRequests: set.New(),
		// knownAccountStatusReplys:   set.New(),
		// knownConMsg:                set.New(),
		// knownPbftMsg:               set.New(),
		NodeType: p.NodeType,
		NodePK:   p.NodePK,
	}
}

// Info gathers and returns a collection of metadata known about a peer.
func (p *peer) Info() *PeerInfo {
	hash, td := p.Head()

	return &PeerInfo{
		Version:    p.version,
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// Head retrieves a copy of the current head hash and total difficulty of the
// peer.
func (p *peer) Head() (hash common.Hash, td *big.Int) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	copy(hash[:], p.head[:])
	//	glog.V(logger.Info).Infoln("p.td %d ", p.td)
	if p.td == nil {
		n := new(big.Int)
		return hash, n
	} else {
		return hash, new(big.Int).Set(p.td)
	}

}

// SetHead updates the head hash and total difficulty of the peer.
func (p *peer) SetHead(hash common.Hash, td *big.Int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	copy(p.head[:], hash[:])
	p.td.Set(td)
}

func (p *peer) BlockHeight() uint64 {
	p.lock.RLock()
	defer p.lock.RUnlock()
	height := p.blockheight
	return height
}
func (p *peer) SetBlockHeight(h uint64) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.blockheight = h
}

// MarkBlock marks a block as known for the peer, ensuring that the block will
// never be propagated to this particular peer.
// func (p *peer) MarkBlock(hash common.Hash) {
// 	// If we reached the memory allowance, drop a previously known block hash
// 	for p.knownBlocks.Size() >= maxKnownBlocks {
// 		p.knownBlocks.Pop()
// 	}
// 	p.knownBlocks.Add(hash)
// }

// MarkTransaction marks a transaction as known for the peer, ensuring that it
// will never be propagated to this particular peer.
// func (p *peer) MarkTransaction(hash common.Hash) {
// 	// If we reached the memory allowance, drop a previously known transaction hash
// 	for p.knownTxs.Size() >= maxKnownTxs {
// 		p.knownTxs.Pop()
// 	}
// 	p.knownTxs.Add(hash)
// }

// add a param PrivateKey
// SendTransactions sends transactions to the peer and includes the hashes
// in its transaction hash set for future reference.
// func (p *peer) SendTransactions(txs TXDM.Transactions, NodePrk *sm2.PrivateKey) error {
// 	for _, tx := range txs {
// 		p.knownTxs.Add(tx.Hash())
// 	}
// 	//	return p2p.Send(p.rw, TxMsg, txs)

// 	data, err := txs.EncodeRlp()
// 	if err != nil {
// 		return err
// 	}

// 	t0 := time.Now()
// 	//压缩交易
// 	var cdata []byte
// 	cdata, err = dencom.Compress(data)
// 	if err != nil {
// 		glog.V(logger.Info).Infoln("compress transactions err :", err)
// 		return err
// 	}

// 	t1 := time.Now()
// 	// 调用 Sign 方法签名
// 	ci, err := Sign(cdata, NodePrk)
// 	if err != nil {
// 		return err
// 	}

// 	if strings.Contains(sc.GetPMLog(), sc.SyncTxs) {
// 		//签名用时
// 		signT := time.Since(t1).Nanoseconds()
// 		compT := t1.UnixNano() - t0.UnixNano() //毫秒
// 		//消息发送时间
// 		glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.TxsMsg,", ci.SigHash().Hex(),
// 			sc.SepChar, p.id[:len(p.id)/16], sc.SepChar, len(txs), sc.SepChar, len(data), sc.SepChar, len(cdata),
// 			sc.SepChar, compT, sc.SepChar, signT, sc.SepChar, time.Now().UnixNano())
// 	}
// 	return p2p.Send(p.rw, TxsMsg, *ci)
// }

// func (p *peer) SendTransaction(tx *TXDM.Transaction, NodePrk *sm2.PrivateKey) error {

// 	p.knownTxs.Add(tx.Hash())

// 	//	return p2p.Send(p.rw, TxMsg, txs)

// 	data, err := tx.EncodeRlp()
// 	if err != nil {
// 		return err
// 	}

// 	// 调用 Sign 方法签名
// 	ci, err := Sign(data, NodePrk)
// 	if err != nil {
// 		return err
// 	}

// 	return p2p.Send(p.rw, TxMsg, *ci)
// }

func signRawData(data interface{}, NodePrk *sm2.PrivateKey) (*CommunicationInfo, error) {
	tdata, err := rlp.EncodeToBytes(data)
	if err != nil {
		return nil, err
	}
	ci, err := Sign(tdata, NodePrk)
	if err != nil {
		return nil, err
	}
	return ci, err
}

// SendNewBlockHashes announces the availability of a number of blocks through
// a hash notification.
// func (p *peer) SendNewBlockHashes(hashes []common.Hash, numbers []uint64, NodePrk *sm2.PrivateKey) error {
// 	for _, hash := range hashes {
// 		p.knownBlocks.Add(hash)
// 	}
// 	request := make(newBlockHashesData, len(hashes))
// 	for i := 0; i < len(hashes); i++ {
// 		request[i].Hash = hashes[i]
// 		request[i].Number = numbers[i]
// 	}

// 	//节点私钥签名
// 	ci, err := signRawData(request, NodePrk)
// 	if err != nil {
// 		return err
// 	}
// 	return p2p.Send(p.rw, NewBlockHashesMsg, *ci)

// }

// SendNewBlock propagates an entire block to a remote peer.
// func (p *peer) SendNewBlock(block *BlockDM.Block, vrs []byte, td *big.Int, NodePrk *sm2.PrivateKey) error {
// 	p.knownBlocks.Add(block.Hash())

// 	data, err := block.EncodeRlp()
// 	if err != nil {
// 		glog.V(logger.Error).Infoln("block.EncodeRlp", err)
// 		return err
// 	}

// 	//压缩区块
// 	var cdata []byte
// 	cdata, err = dencom.Compress(data)
// 	if err != nil {
// 		glog.V(logger.Info).Infoln("compress block err :", err)
// 		return err
// 	}

// 	// 调用 Sign 方法签名
// 	ci, err := Sign(cdata, NodePrk)
// 	if err != nil {
// 		glog.V(logger.Error).Infoln("Sign blockData Err", err)
// 		return err
// 	}

// 	ci.VRS = vrs

// 	return p2p.Send(p.rw, NewBlockMsg, []interface{}{*ci, td})
// }

// SendBlockHeaders sends a batch of block headers to the remote peer.
// func (p *peer) SendBlockHeaders(headers []*HeaderWithVRS.HeaderWithVRS, vrss [][]byte, NodePrk *sm2.PrivateKey) error {
// 	//	glog.V(logger.Info).Infof("send headers: %d", headers)
// 	//datas := HeaderDM.HeadersForStorage{}
// 	var datas HeaderWithVRS.HeadersWithVRSForStorage
// 	for _, header := range headers {
// 		data, err := header.EncodeRlp()
// 		if err != nil {
// 			return err
// 		}
// 		//datas[inx] = data
// 		datas = append(datas, data)
// 	}

// 	//节点私钥签名
// 	ci, err := signRawData(datas, NodePrk)
// 	if err != nil {
// 		return err
// 	}

// 	var vrssdata VRS.VRSSForStorage
// 	for _, val := range vrss {
// 		vrssdata = append(vrssdata, val)
// 	}
// 	vrssbyte, err := rlp.EncodeToBytes(vrss)
// 	if err != nil {
// 		return err
// 	}
// 	ci.VRS = vrssbyte
// 	if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 		//签名用时
// 		origin := big.NewInt(0)
// 		if len(headers) != 0 {
// 			origin = headers[0].Header.GetNumber()
// 		}
// 		tdata, _ := rlp.EncodeToBytes(datas)
// 		glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.BlockHeadersMsg,", ci.SigHash().Hex(),
// 			sc.SepChar, p.id[:len(p.id)/16], sc.SepChar, origin, sc.SepChar, len(headers),
// 			sc.SepChar, len(tdata), sc.SepChar, time.Now().UnixNano())
// 	}
// 	return p2p.Send(p.rw, BlockHeadersMsg, *ci)
// }

/*
// SendBlockBodies sends a batch of block contents to the remote peer.
func (p *peer) SendBlockBodies(bodies []*blockBody, NodePrk *sm2.PrivateKey) error {
	//datas := BlockDM.BodiesForStorage{}
	var datas BlockDM.BodiesForStorage
	for _, b := range bodies {
		body := BlockDM.Body{
			Transactions: b.Transactions,
			Uncles:       b.Uncles,
		}
		data, err := body.EncodeRlp()
		if err != nil {
			return err
		}
		//datas[inx] = data
		datas = append(datas, data)
	}

	//节点私钥签名
	ci, err := signRawData(datas, NodePrk)
	if err != nil {
		return err
	}
	return p2p.Send(p.rw, BlockBodiesMsg, *ci)
}
*/

// RlpAndCompressAndSign，bodys、receipts等需要压缩的原始数据使用
// 先rlp编码、再压缩、再签名
// func RlpAndCompressAndSign(data interface{}, NodePrk *sm2.PrivateKey) (*CommunicationInfo, error) {
// 	//rlp编码
// 	rlpdata, err := rlp.EncodeToBytes(data)
// 	if err != nil {
// 		return nil, err
// 	}

// 	t0 := time.Now()
// 	//压缩
// 	cdata, err := dencom.Compress(rlpdata)
// 	if err != nil {
// 		glog.V(logger.Info).Infoln("compress block err :", err)
// 		return nil, err
// 	}
// 	t1 := time.Now()
// 	//节点私钥签名
// 	ci, err := Sign(cdata, NodePrk)
// 	if err != nil {
// 		return nil, err
// 	}
// 	//有时候可能不需要解压缩时间，与区块同步做区分
// 	if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 		signT := time.Since(t0).Nanoseconds()
// 		cmpresT := t1.UnixNano() - t0.UnixNano()
// 		glog.V(logger.Info).Infoln("pm.peer.Send.BlockBodiesOrReceipts.RlpAndCompressAndSign,",
// 			ci.SigHash().Hex(), sc.SepChar, len(rlpdata), sc.SepChar, len(cdata), sc.SepChar, cmpresT, sc.SepChar, signT)
// 	}
// 	return ci, err
// }

// SendBlockBodiesRLP sends a batch of block contents to the remote peer from
// an already RLP encoded format.
// func (p *peer) SendBlockBodiesRLP(bodies []rlp.RawValue, NodePrk *sm2.PrivateKey) error {
// 	var datas BlockDM.BodiesForStorage
// 	for _, val := range bodies {
// 		datas = append(datas, val)
// 	}
// 	//节点私钥签名
// 	ci, err := RlpAndCompressAndSign(datas, NodePrk)
// 	if err != nil {
// 		return err
// 	}
// 	if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 		glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.BlockBodiesMsg,",
// 			ci.SigHash().Hex(), sc.SepChar, p.id[:len(p.id)/16],
// 			sc.SepChar, len(bodies), sc.SepChar, time.Now().UnixNano())
// 	}
// 	return p2p.Send(p.rw, BlockBodiesMsg, *ci)
// }

// SendNodeDataRLP sends a batch of arbitrary internal data, corresponding to the
// hashes requested.
// func (p *peer) SendNodeData(data [][]byte, NodePrk *sm2.PrivateKey) error {

// 	//节点私钥签名
// 	ci, err := signRawData(data, NodePrk)
// 	if err != nil {
// 		return err
// 	}
// 	return p2p.Send(p.rw, NodeDataMsg, *ci)
// }

// SendReceiptsRLP sends a batch of transaction receipts, corresponding to the
// ones requested from an already RLP encoded format.
// func (p *peer) SendReceiptsRLP(receipts []rlp.RawValue, NodePrk *sm2.PrivateKey) error {

// 	//节点私钥签名
// 	ci, err := RlpAndCompressAndSign(receipts, NodePrk)
// 	if err != nil {
// 		return err
// 	}

// 	if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 		glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.ReceiptsMsg,",
// 			ci.SigHash().Hex(), sc.SepChar, p.id[:len(p.id)/16],
// 			sc.SepChar, len(receipts), sc.SepChar, time.Now().UnixNano())
// 	}
// 	return p2p.Send(p.rw, ReceiptsMsg, *ci)
// }

// func (p *peer) RequestOneTransaction(hash common.Hash, NodePrk *sm2.PrivateKey) error {
// 	if p.NodeType == uint64(4) {
// 		return nil
// 	}
// 	glog.V(logger.Debug).Infof("%v fetching one transaction", p, hash.Hex())

// 	//节点私钥签名
// 	ci, err := Sign(hash[:], NodePrk)
// 	if err != nil {
// 		return err
// 	}

// 	return p2p.Send(p.rw, GetTxMsg, *ci)
// }

// RequestHeaders is a wrapper around the header query functions to fetch a
// single header. It is used solely by the fetcher.
// func (p *peer) RequestOneHeader(hash common.Hash, NodePrk *sm2.PrivateKey) error {
// 	if p.NodeType == uint64(1) {
// 		glog.V(logger.Debug).Infof("%v fetching a single header: %x", p, hash)

// 		data := &getBlockHeadersData{
// 			Origin:  hashOrNumber{Hash: hash},
// 			Amount:  uint64(1),
// 			Skip:    uint64(0),
// 			Reverse: false,
// 		}

// 		//节点私钥签名
// 		ci, err := signRawData(data, NodePrk)
// 		if err != nil {
// 			return err
// 		}

// 		return p2p.Send(p.rw, GetBlockHeadersMsg, *ci)

// 	}
// 	return nil
// }

// RequestHeadersByHash fetches a batch of blocks' headers corresponding to the
// specified header query, based on the hash of an origin block.
// func (p *peer) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool, NodePrk *sm2.PrivateKey) error {
// 	if p.NodeType == uint64(1) {
// 		glog.V(logger.Debug).Infof("%v fetching %d headers from %x, skipping %d (reverse = %v)", p, amount, origin[:4], skip, reverse)

// 		data := &getBlockHeadersData{
// 			Origin:  hashOrNumber{Hash: origin},
// 			Amount:  uint64(amount),
// 			Skip:    uint64(skip),
// 			Reverse: reverse,
// 		}

// 		//节点私钥签名
// 		ci, err := signRawData(data, NodePrk)
// 		if err != nil {
// 			return err
// 		}
// 		if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 			glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.GetBlockHeadersMsgByHash,", ci.SigHash().Hex(),
// 				sc.SepChar, p.id[:len(p.id)/16], sc.SepChar, origin.Hex(), sc.SepChar, amount, sc.SepChar, time.Now().UnixNano())
// 		}
// 		return p2p.Send(p.rw, GetBlockHeadersMsg, *ci)
// 	}
// 	return nil
// }

// RequestHeadersByNumber fetches a batch of blocks' headers corresponding to the
// specified header query, based on the number of an origin block.
// func (p *peer) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool, NodePrk *sm2.PrivateKey) error {
// 	if p.NodeType == uint64(1) {
// 		glog.V(logger.Debug).Infof("%v fetching %d headers from #%d, skipping %d (reverse = %v)", p, amount, origin, skip, reverse)

// 		data := &getBlockHeadersData{
// 			Origin:  hashOrNumber{Number: origin},
// 			Amount:  uint64(amount),
// 			Skip:    uint64(skip),
// 			Reverse: reverse,
// 		}

// 		//节点私钥签名
// 		ci, err := signRawData(data, NodePrk)
// 		if err != nil {
// 			return err
// 		}
// 		if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 			glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.GetBlockHeadersMsgByNum,", ci.SigHash().Hex(),
// 				sc.SepChar, p.id[:len(p.id)/16], sc.SepChar, origin, sc.SepChar, amount, sc.SepChar, time.Now().UnixNano())
// 		}
// 		return p2p.Send(p.rw, GetBlockHeadersMsg, *ci)
// 	}
// 	return nil

// }

// RequestBodies fetches a batch of blocks' bodies corresponding to the hashes
// specified.
// func (p *peer) RequestBodies(hashes []common.Hash, NodePrk *sm2.PrivateKey) error {
// 	if p.NodeType == uint64(4) {
// 		return nil
// 	}
// 	glog.V(logger.Debug).Infof("%v fetching %d block bodies", p, len(hashes))

// 	//节点私钥签名
// 	ci, err := signRawData(hashes, NodePrk)
// 	if err != nil {
// 		return err
// 	}
// 	if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 		glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.GetBlockBodiesMsg,",
// 			ci.SigHash().Hex(), sc.SepChar, p.id[:len(p.id)/16],
// 			sc.SepChar, len(hashes), sc.SepChar, time.Now().UnixNano())
// 	}
// 	return p2p.Send(p.rw, GetBlockBodiesMsg, *ci)
// }

// RequestNodeData fetches a batch of arbitrary data from a node's known state
// data, corresponding to the specified hashes.
// func (p *peer) RequestNodeData(hashes []common.Hash, NodePrk *sm2.PrivateKey) error {

// 	glog.V(logger.Debug).Infof("%v fetching %v state data", p, len(hashes))

// 	//节点私钥签名
// 	ci, err := signRawData(hashes, NodePrk)
// 	if err != nil {
// 		return err
// 	}
// 	return p2p.Send(p.rw, GetNodeDataMsg, *ci)
// }

// RequestReceipts fetches a batch of transaction receipts from a remote node.
// func (p *peer) RequestReceipts(hashes []common.Hash, NodePrk *sm2.PrivateKey) error {
// 	glog.V(logger.Debug).Infof("%v fetching %v receipts", p, len(hashes))

// 	//节点私钥签名
// 	ci, err := signRawData(hashes, NodePrk)
// 	if err != nil {
// 		return err
// 	}
// 	if strings.Contains(sc.GetPMLog(), sc.SyncBlocks) {
// 		glog.V(logger.Info).Infoln(sc.TestPrefix, "pm.peer.Send.GetReceiptsMsg,",
// 			ci.SigHash().Hex(), sc.SepChar, p.id[:len(p.id)/16],
// 			sc.SepChar, len(hashes), sc.SepChar, time.Now().UnixNano())
// 	}
// 	return p2p.Send(p.rw, GetReceiptsMsg, *ci)
// }

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks. Handshake 执行 eth 协议握手，协商版本号、网络 ID、难度、头部和创世块
// func (p *peer) Handshake(network int, td *big.Int, head common.Hash, genesis common.Hash) error {
// 	// Send out own handshake in a new thread
// 	errc := make(chan error, 2)
// 	var status statusData // safe to read after two values have been received from errc 从 errc 收到两个值后可以安全读取

// 	go func() {
// 		errc <- p2p.Send(p.rw, StatusMsg, &statusData{
// 			ProtocolVersion: uint32(p.version),
// 			NetworkId:       uint32(network),
// 			TD:              td,
// 			CurrentBlock:    head,
// 			GenesisBlock:    genesis,
// 		})
// 	}()
// 	go func() {
// 		errc <- p.readStatus(network, &status, genesis)
// 	}()
// 	timeout := time.NewTimer(handshakeTimeout)
// 	defer timeout.Stop()
// 	for i := 0; i < 2; i++ {
// 		select {
// 		case err := <-errc:
// 			if err != nil {
// 				return err
// 			}
// 		case <-timeout.C:
// 			return p2p.DiscReadTimeout
// 		}
// 	}
// 	p.td, p.head = status.TD, status.CurrentBlock
// 	return nil
// }

// func (p *peer) readStatus(network int, status *statusData, genesis common.Hash) (err error) {
// 	msg, err := p.rw.ReadMsg()
// 	if err != nil {
// 		return err
// 	}
// 	if msg.Code != StatusMsg {
// 		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, StatusMsg)
// 	}
// 	if msg.Size > ProtocolMaxMsgSize {
// 		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
// 	}
// 	// Decode the handshake and make sure everything matches
// 	if err := msg.Decode(&status); err != nil {
// 		return errResp(ErrDecode, "msg %v: %v", msg, err)
// 	}
// 	if status.GenesisBlock != genesis {
// 		return errResp(ErrGenesisBlockMismatch, "%x (!= %x)", status.GenesisBlock, genesis)
// 	}
// 	if int(status.NetworkId) != network {
// 		return errResp(ErrNetworkIdMismatch, "%d (!= %d)", status.NetworkId, network)
// 	}
// 	if int(status.ProtocolVersion) != p.version {
// 		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, p.version)
// 	}
// 	return nil
// }

// String implements fmt.Stringer.
func (p *peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.id,
		fmt.Sprintf("eth/%2d", p.version),
	)
}

// peerSet represents the collection of active peers currently participating in
// the Ethereum sub-protocol.
type peerSet struct {
	peers sync.Map //map[string]*peer
	//lock   sync.RWMutex
	closed bool
}

// add new function : GetPeers() to get the peerSet.peers
//func (ps *peerSet) GetPeers() sync.Map/*map[string]*peer*/ {
//	return ps.peers
//}

func (ps *peerSet) GetPeers() []*peer {
	list := make([]*peer, 0)
	ps.peers.Range(func(k, v interface{}) bool {
		p := v.(*peer)
		list = append(list, p)
		return true
	})
	return list
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: sync.Map{}, //make(map[string]*peer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known. Register 向工作集中注入一个新的对等点，如果对等点已知，则返回错误。
func (ps *peerSet) Register(p *peer) error {

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers.Load(p.id); ok {
		return errAlreadyRegistered
	}
	ps.peers.Store(p.id, p)
	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(id string) error {

	if _, ok := ps.peers.Load(id); !ok {
		return errNotRegistered
	}
	ps.peers.Delete(id)
	return nil
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id string) *peer {
	val, ok := ps.peers.Load(id)
	if ok {
		return val.(*peer)
	}
	return nil
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	return lenOfSyncMap(ps.peers)
}

func lenOfSyncMap(val sync.Map) int {
	len := 0
	val.Range(func(k, v interface{}) bool {
		len++
		return true
	})
	return len
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
// func (ps *peerSet) PeersWithoutBlock(hash common.Hash) []*peer {
// 	list := make([]*peer, 0, lenOfSyncMap(ps.peers))
// 	ps.peers.Range(func(k, v interface{}) bool {
// 		p := v.(*peer)
// 		if !p.knownBlocks.Has(hash) {
// 			list = append(list, p)
// 		}
// 		return true
// 	})
// 	return list
// }

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
// func (ps *peerSet) PeersWithoutTx(hash common.Hash) []*peer {

// 	list := make([]*peer, 0, lenOfSyncMap(ps.peers))
// 	ps.peers.Range(func(k, v interface{}) bool {
// 		p := v.(*peer)
// 		if !p.knownTxs.Has(hash) {
// 			list = append(list, p)
// 		}
// 		return true
// 	})
// 	return list
// }

// BestPeer 是指当前难度值最大的节点
// BestPeer retrieves the known peer with the currently highest total difficulty.
// func (ps *peerSet) BestPeer() *peer {

// 	var (
// 		bestPeer *peer
// 		bestTd   *big.Int
// 	)

// 	ps.peers.Range(func(k, v interface{}) bool {
// 		p := v.(*peer)
// 		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
// 			if p.NodeType == uint64(1) { // 未知
// 				bestPeer, bestTd = p, td
// 			}
// 		}
// 		return true
// 	})

// 	/*
// 	* to judge peer is null
// 	 */
// 	if bestPeer == nil {
// 		ps.peers.Range(func(k, v interface{}) bool {
// 			p := v.(*peer)
// 			if _, td := p.Head(); bestPeer == nil /*|| td.Cmp(bestTd) >= 0 */ {
// 				if p.NodeType == uint64(1) { // 未知
// 					bestPeer, bestTd = p, td
// 				}
// 				//				bestPeer, bestTd = p, td
// 			}
// 			return true
// 		})
// 	}

// 	return bestPeer
// }

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *peerSet) Close() {

	ps.peers.Range(func(k, v interface{}) bool {
		p := v.(*peer)
		p.Disconnect(p2p.DiscQuitting)
		return true
	})
	ps.closed = true
}

//~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~//

/*
***	下面的所有的Mark函数，可以阻止信息的重复广播
***	但是会引发，某些正常信息无法发送
*** 例如：本节点收到来自其他节点的消息，会与本节点信息冲突，本节点消息无法发送
*** 解决方案结合ReplicaId同时作为标识
 */
// MarkRequest marks a request as known for the peer, ensuring that it
// will never be propagated to this particular peer.
//func (p *peer) MarkRequest(req core.Request) { // core.Request应该具有唯一标识，暂时使用Content代替
//	for p.knownRequests.Size() >= maxKnownTxs {
//		p.knownRequests.Pop()
//	}
//	p.knownRequests.Add(req.TxHash)
//}
//
//func (p *peer) MarkPrePrepare(prepre core.PrePrepare_out) { // core.PrePrepare应该具有唯一标识，暂时使用RequestDigest代替
//	for p.knownPrePrepares.Size() >= maxKnownTxs {
//		p.knownPrePrepares.Pop()
//	}
//	//	string := strconv.Itoa(prepre.ReplicaId)
//	p.knownPrePrepares.Add(prepre.RequestDigest + fmt.Sprintf("%d", prepre.ReplicaId))
//}
//
//func (p *peer) MarkPrepare(pre core.Prepare_out) { // core.Prepare应该具有唯一标识，暂时使用RequestDigest代替
//	for p.knownPrepares.Size() >= maxKnownTxs {
//		p.knownPrepares.Pop()
//	}
//	p.knownPrepares.Add(pre.RequestDigest + fmt.Sprintf("%d", pre.ReplicaId))
//}
//
//func (p *peer) MarkCommit(comt core.Commit_out) { // core.Commit应该具有唯一标识，暂时使用RequestDigest代替
//	for p.knownCommits.Size() >= maxKnownTxs {
//		p.knownCommits.Pop()
//	}
//	p.knownCommits.Add(comt.RequestDigest + fmt.Sprintf("%d", comt.ReplicaId))
//}
//
//func (p *peer) MarkCheckPoint(chk core.Checkpoint_out) { // core.Checkpoint 应该具有唯一标识，暂时使用RequestDigest代替
//	for p.knownCheckpoints.Size() >= maxKnownTxs {
//		p.knownCheckpoints.Pop()
//	}
//	p.knownCheckpoints.Add(chk.BlockHash.Str() + fmt.Sprintf("%d", chk.ReplicaId))
//}
//
//func (p *peer) MarkNewView(nv core.NewView_out) { // core.NewView 应该具有唯一标识，暂时使用 view 代替
//	for p.knownNewViews.Size() >= maxKnownTxs {
//		p.knownNewViews.Pop()
//	}
//	p.knownNewViews.Add(fmt.Sprintf("%d", nv.View) + fmt.Sprintf("%d", nv.ReplicaId))
//}
//
//func (p *peer) MarkViewChange(vc core.ViewChange_out) { // core.ViewChange 应该具有唯一标识，暂时使用 view 代替
//	for p.knownViewChanges.Size() >= maxKnownTxs {
//		p.knownViewChanges.Pop()
//	}
//	p.knownViewChanges.Add(fmt.Sprintf("%d", vc.View) + fmt.Sprintf("%d", vc.ReplicaId))
//}

//func (p *peer) MarkAccountStatusRequest(vc *core.AccountStatusRequest) { // core.ViewChange 应该具有唯一标识，暂时使用 view 代替
//	for p.knownAccountStatusRequests.Size() >= maxKnownTxs {
//		p.knownAccountStatusRequests.Pop()
//	}
//	p.knownAccountStatusRequests.Add(vc.RequestAccount)
//}
//func (p *peer) MarkAccountStatusReply(vc *core.AccountStatusReply) { // core.ViewChange 应该具有唯一标识，暂时使用 view 代替
//	for p.knownAccountStatusReplys.Size() >= maxKnownTxs {
//		p.knownAccountStatusReplys.Pop()
//	}
//	p.knownAccountStatusReplys.Add(vc.RequestAccount)
//}

// func (p *peer) MarkConMsg(vc *core.ConsensusMsg) {
// 	for p.knownConMsg.Size() >= maxKnownTxs {
// 		p.knownConMsg.Pop()
// 	}
// 	p.knownConMsg.Add(fmt.Sprintf("%d", vc.Timestamp) + fmt.Sprintf("%d", vc.Sender))
// }

// 20190823 yili
//func (p *peer) MarkPbftMsg(vc *core.PbftMessage) {
//	for p.knownPbftMsg.Size() >= maxKnownTxs {
//		p.knownPbftMsg.Pop()
//	}
//	p.knownPbftMsg.Add(fmt.Sprintf("%d", vc.Timestamp) + fmt.Sprintf("%d", vc.Sender))
//}

// PeersWithoutRequest retrieves a list of peers that do not have a given request
// in their set of known hashes.
//func (ps *peerSet) PeersWithoutRequest(req *core.Request) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		if !p.knownRequests.Has(req.TxHash) {
//			list = append(list, p)
//		}
//	}
//	return list
//}
//
//func (ps *peerSet) PeersWithoutPrePrepare(prepre *core.PrePrepare_out) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		//		if !p.knownPrePrepares.Has(prepre.RequestDigest + fmt.Sprintf("%d", prepre.ReplicaId)) {
//		list = append(list, p)
//		//		}
//	}
//	return list
//}
//
//func (ps *peerSet) PeersWithoutPrepare(pre *core.Prepare_out) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		//		if !p.knownPrepares.Has(pre.RequestDigest + fmt.Sprintf("%d", pre.ReplicaId)) {
//		list = append(list, p)
//		//		}
//	}
//	return list
//}
//
//func (ps *peerSet) PeersWithoutCommit(comt *core.Commit_out) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		//		if !p.knownCommits.Has(comt.RequestDigest + fmt.Sprintf("%d", comt.ReplicaId)) {
//		list = append(list, p)
//		//		}
//	}
//	return list
//}
//
//func (ps *peerSet) PeersWithoutCheckpoint(chk *core.Checkpoint_out) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		//		if !p.knownCheckpoints.Has(chk.BlockHash.Str() + fmt.Sprintf("%d", chk.ReplicaId)) {
//		list = append(list, p)
//		//		}
//	}
//	return list
//}
//
//func (ps *peerSet) PeersWithoutNewView(nv *core.NewView_out) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		//		if !p.knownNewViews.Has(fmt.Sprintf("%d", nv.View) + fmt.Sprintf("%d", nv.ReplicaId)) {
//		list = append(list, p)
//		//		}
//	}
//	return list
//}
//
//func (ps *peerSet) PeersWithoutViewChange(vc *core.ViewChange_out) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		//		if !p.knownViewChanges.Has(fmt.Sprintf("%d", vc.View) + fmt.Sprintf("%d", vc.ReplicaId)) {
//		list = append(list, p)
//		//		}
//	}
//	return list
//}

//func (ps *peerSet) PeersWithoutAccountStatusRequest(vc *core.AccountStatusRequest) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		if !p.knownAccountStatusRequests.Has(vc.RequestAccount) {
//			list = append(list, p)
//		}
//	}
//	return list
//}
//
//func (ps *peerSet) PeersWithoutAccountStatusReply(vc *core.AccountStatusReply) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		if !p.knownAccountStatusReplys.Has(vc.RequestAccount) {
//			list = append(list, p)
//		}
//	}
//	return list
//}

// PeersWithoutConMsg
// func (ps *peerSet) PeersWithoutConMsg(vc *core.ConsensusMsg) []*peer {

// 	list := make([]*peer, 0, lenOfSyncMap(ps.peers))
// 	ps.peers.Range(func(k, v interface{}) bool {
// 		p := v.(*peer)
// 		list = append(list, p)
// 		return true
// 	})
// 	//	glog.V(logger.Info).Infoln("peers list is  %d ", len(list))
// 	return list
// }

// PeersWithoutPbftMsg
//func (ps *peerSet) PeersWithoutPbftMsg(vc *core.PbftMessage) []*peer {
//	ps.lock.RLock()
//	defer ps.lock.RUnlock()
//
//	list := make([]*peer, 0, len(ps.peers))
//	for _, p := range ps.peers {
//		list = append(list, p)
//	}
//	//	glog.V(logger.Info).Infoln("peers list is  %d ", len(list))
//	return list
//}

// SendRequest sends Request to the peer and includes the hashes
// in its transaction hash set for future reference.
//func (p *peer) SendRequest(req *core.Request, NodePrk *sm2.PrivateKey) error {
//
//	p.knownRequests.Add(req.TxHash)
//
//	//节点私钥签名
//	ci, err := signRawData(req, NodePrk)
//	if err != nil {
//		return err
//	}
//	return p2p.Send(p.rw, RequsetMsg, *ci)
//}
//
//func (p *peer) SendPrePrepare(prepre *core.PrePrepare_out, NodePrk *sm2.PrivateKey) error {
//
//	p.knownPrePrepares.Add(prepre.RequestDigest + fmt.Sprintf("%d", prepre.ReplicaId))
//
//	//节点私钥签名
//	ci, err := signRawData(prepre, NodePrk)
//	if err != nil {
//		return err
//	}
//	return p2p.Send(p.rw, PrePrepareMsg, *ci)
//}
//
//func (p *peer) SendPrepare(pre *core.Prepare_out, NodePrk *sm2.PrivateKey) error {
//
//	p.knownPrepares.Add(pre.RequestDigest + fmt.Sprintf("%d", pre.ReplicaId))
//
//	//节点私钥签名
//	ci, err := signRawData(pre, NodePrk)
//	if err != nil {
//		return err
//	}
//	return p2p.Send(p.rw, PrepareMsg, *ci)
//}
//
//func (p *peer) SendCommit(comt *core.Commit_out, NodePrk *sm2.PrivateKey) error {
//
//	p.knownCommits.Add(comt.RequestDigest + fmt.Sprintf("%d", comt.ReplicaId))
//
//	//节点私钥签名
//	ci, err := signRawData(comt, NodePrk)
//	if err != nil {
//		return err
//	}
//	return p2p.Send(p.rw, CommitMsg, *ci)
//}
//
//func (p *peer) SendCheckpoint(chk *core.Checkpoint_out, NodePrk *sm2.PrivateKey) error {
//
//	p.knownCheckpoints.Add(chk.BlockHash.Str() + fmt.Sprintf("%d", chk.ReplicaId))
//
//	//节点私钥签名
//	ci, err := signRawData(chk, NodePrk)
//	if err != nil {
//		return err
//	}
//	return p2p.Send(p.rw, CheckpointMsg, *ci)
//}
//
//func (p *peer) SendNewView(nv *core.NewView_out, NodePrk *sm2.PrivateKey) error {
//
//	p.knownNewViews.Add(fmt.Sprintf("%d", nv.View) + fmt.Sprintf("%d", nv.ReplicaId))
//
//	//节点私钥签名
//	ci, err := signRawData(nv, NodePrk)
//	if err != nil {
//		return err
//	}
//	return p2p.Send(p.rw, NewViewMsg, *ci)
//}
//
//func (p *peer) SendViewChange(vc *core.ViewChange_out, NodePrk *sm2.PrivateKey) error {
//
//	p.knownViewChanges.Add(fmt.Sprintf("%d", vc.View) + fmt.Sprintf("%d", vc.ReplicaId))
//
//	//节点私钥签名
//	ci, err := signRawData(vc, NodePrk)
//	if err != nil {
//		return err
//	}
//
//	return p2p.Send(p.rw, ViewChangeMsg, *ci)
//}

//已删除
//func (p *peer) SendAccountStatusRequest(vc *core.AccountStatusRequest) error {
//
//	return p2p.Send(p.rw, AccountStatusRequestMsg, vc)
//}

//已删除
//func (p *peer) SendAccountStatusReply(vc *core.AccountStatusReply) error {
//
//	//	p.knownAccountStatusReplys.Add(vc.RequestAccount)
//	return p2p.Send(p.rw, AccountStatusReplyMsg, vc)
//}

func (p *peer) SendTestSendMsg(vc *TestMsg, NodePrk *sm2.PrivateKey) error {

	//	p.knownAccountStatusRequests.Add(vc.RequestAccount)

	//节点私钥签名
	ci, err := signRawData(vc, NodePrk)
	if err != nil {
		return err
	}
	// fmt.Println("发送SendMsg")测试用
	return p2p.Send(p.rw, TestSendMsg, *ci)
}

func (p *peer) SendTestReciveMsg(vc *TestMsg, NodePrk *sm2.PrivateKey) error {

	//	p.knownAccountStatusRequests.Add(vc.RequestAccount)

	//节点私钥签名
	ci, err := signRawData(vc, NodePrk)
	if err != nil {
		return err
	}
	return p2p.Send(p.rw, TestReciveMsg, *ci)
}

func (p *peer) SendConsSendMsg(vc *ConsensusMsg, NodePrk *sm2.PrivateKey) error {

	//	p.knownAccountStatusRequests.Add(vc.RequestAccount)

	//节点私钥签名
	ci, err := signRawData(vc, NodePrk)
	if err != nil {
		return err
	}
	// fmt.Println("发送SendMsg")测试用
	return p2p.Send(p.rw, ConMsg, *ci)
}

func (p *peer) SendCheckPointMsg(vc *Checkpoint, NodePrk *sm2.PrivateKey) error {

	//	p.knownAccountStatusRequests.Add(vc.RequestAccount)

	//节点私钥签名
	ci, err := signRawData(vc, NodePrk)
	if err != nil {
		return err
	}
	// fmt.Println("发送SendMsg")测试用
	return p2p.Send(p.rw, Checkpoin, *ci)
}
func (p *peer) SendZhuanfaMsg(vc *ConsensusMsg2, NodePrk *sm2.PrivateKey) error {

	//	p.knownAccountStatusRequests.Add(vc.RequestAccount)

	//节点私钥签名
	ci, err := signRawData(vc, NodePrk)
	if err != nil {
		return err
	}
	// fmt.Println("发送SendMsg")测试用
	return p2p.Send(p.rw, Zhuanfa, *ci)
}

// fixme
//func (p *peer) SendPbftMsg(vc *core.PbftMessage, NodePrk *sm2.PrivateKey) error {
//
//	//	p.knownAccountStatusRequests.Add(vc.RequestAccount)
//
//	//节点私钥签名
//	ci, err := signRawData(vc, NodePrk)
//	if err != nil {
//		return err
//	}
//
//	return p2p.Send(p.rw, PbftMsg, *ci)
//}
