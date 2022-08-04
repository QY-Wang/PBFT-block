package eth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DWBC-ConPeer/core/dencom"

	//"sort"
	"strconv"

	//"github.com/DWBC-ConPeer/core/VRS"
	//"github.com/pkg/profile"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DWBC-ConPeer/common"
	//"github.com/DWBC-ConPeer/core"
	//"github.com/DWBC-ConPeer/core/types/BlockDM"
	//"github.com/DWBC-ConPeer/core/types/HeaderDM"

	//"github.com/DWBC-ConPeer/core/types/TXDM"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	//sc "github.com/DWBC-ConPeer/systemconfig"
)

const (
	possitive       = 0
	futurePossitive = 1
	negative        = 2
)

var Heightlock sync.RWMutex

// 仅仅执行许可节点之间的共识
var maxConsensusLifetime = time.Second * 40 // 两分钟
var evictionInterval = time.Minute * 1

const TermNum uint64 = 1000          // 一轮可以构建的区块数量
const TermTime = time.Hour * 24      //一轮可以持续的时间，优先级低于区块高度
var leaseBeat = time.Now()           //租约起始时间，一轮共识完成，收到checkPoint会更新，setConstatus会更新
var lbMux sync.RWMutex               // leaseBeat 的读写锁
var chkptGroup = new(sync.WaitGroup) // checkpt 协程协调器，保证同一时刻有且仅有一个RecvCheckpoint函数进行
// Note: 考虑一个问题，是否可以所有的共识过程均通过waitgroup管理
// 但会将并发过程强制转为串行过程
var consensusCtx context.Context          // guarantee one startconsensus at the same time
var consensusCancel context.CancelFunc    // guarantee one startconsensus at the same time
var checkpointStore sync.Map              //map[common.Hash]uint64 区块hash，区块高度
var checkpointBH uint64                   // 执行过checkPoint的最大区块高度，注意启动时的处理 //当前高度是相等的，当时checpoint Store相等
var chkMux sync.RWMutex                   //checkpointBH的读写锁
var commitStore = make(map[uint64]string) //执行过checkpoint的区块维护，只维护最高的
var commitMux sync.RWMutex                // commit 的读写锁
//改：

var prepreflag, preflag, commitflag = 0, 0, 0

type Block struct {
	Number  uint64 //区块高度
	Valid   int    //区块通过或者不通过
	Timee   time.Time
	Header  *Header
	Txs     []*Transaction
	Uncles  []*Header
	Receipt []*Receipt
}

func (self *Block) Hash() string {
	return strconv.Itoa(int(self.Number))
}
func (self *Block) Time() time.Time {
	return self.Timee
}

var Txs = []*Transaction{nil}
var Uncles = []*Header{nil}
var Receipts = []*Receipt{nil}

//注意使用一个静态的block当作当前block，此block的高度应该和chain.currentblock.number()保持一致
var currentblock = Block{1, 1, time.Now(), nil, Txs, Uncles, Receipts}

//区块是否通过
func is_valid(block *Block) bool {
	if block.Valid == 0 {
		return true
	}
	return false
}

type TxsExeSequences struct {
	//交易数组和交易依赖数组组合起来
	Objects_ExeSeqs [50]int
	Dependency_TX   [50]int
}

func (self *Block) Transactions() (string, [50]int, [50]int) {
	//自己加的函数
	str := "一个返回值，但是用不到"
	var objects_ExeSeqs [50]int
	var dependency_TX [50]int
	//dependency_TX和object_ExeSeqs分别为交易序列和交易依赖，这里暂时使用两个数组表示
	for i := 0; i < 50; i++ {
		objects_ExeSeqs[i] = i + 1
	}
	for i := 0; i < 50; i++ {
		dependency_TX[i] = 50 - i
	}
	return str, objects_ExeSeqs, dependency_TX
}

func TxDependency(a string, b, c [50]int) (string, [50]int, [50]int) {
	//自己改的函数使代码看起来不突兀........
	return a, b, c
}

type Request struct {
	ReqID        uint64 /*self.chain.CurrentBlock().NumberU64()+1,*/ //FIXME
	Sender       string
	View         uint64
	Timestamp    time.Time
	BlockHeight  uint64
	BlockHash    string //暂定hash值为string类型
	BlockCreator common.Address
	Block        []byte
	//新创造的，不使用解码获得block，直接获取
	Realblock *Block

	//TxsExeSeqs:   txExec,
	// Sig 暂时不写
}

func newhashReq(req *Request) string {
	return strconv.Itoa(int(req.ReqID))
}

type ConsensusMsg struct {
	Msg       string //self.CompressAndTostring(req), //编码并压缩
	Timestamp time.Time
	Type      string
	Sender    string
}
type ConsensusMsg2 struct {
	//转发
	Msg       string //self.CompressAndTostring(req), //编码并压缩
	Timestamp time.Time
	Type      string
	Sender    string
	Newsender string
	Other     string
}

type PrePrepare struct {
	Sender        string
	View          uint64
	Timestamp     time.Time
	ReqID         uint64
	RequestDigest string
	Request       *Request
	// FIXME Sig 暂时不做处理
}

type Prepare struct {
	Sender        string //instance.GetSelfNodeID(),
	View          uint64
	Timestamp     time.Time
	ReqID         uint64 //preprep.ReqID,
	RequestDigest string //preprep.RequestDigest,
	TxsExeSeqs    []byte //不再使用,也不知道类型，先用string保留着吧,是[]byte类型
	//使用未压缩的交易组合
	RealTxsExeSeqs *TxsExeSequences
}

type ValidResultForBlock struct {
	Sender       string
	BlockHeight  uint64
	BlockHash    string
	BlockCreator common.Address
	Opinion      bool
	Sign         string //进行签名
}

func (self *worker) vrsign(vr *ValidResultForBlock) bool {
	var a = true
	if a {
		vr.Sign = self.nodeID
		return true
	}
	return false
}

type Commit struct {
	Sender        string
	View          uint64
	Timestamp     time.Time
	ReqID         uint64
	RequestDigest string
	VR            ValidResultForBlock
}

type Checkpoint struct {
	Sender      string
	BlockHeight uint64
	ReqID       uint64
	BlockHash   string
}

type NewMinedBlockEvent struct {
	Block *Block
	VRS   string //暂不清楚类型
}

func InsertChainWithVRS() {
	fmt.Println("插入success")
}

type Header struct {
	ParentHash string
	Number     *big.Int
	Coinbase   common.Address
	Extra      []byte
	Time       *big.Int
	ChainName  string
	Root       string
}

func ReSetReceipts(a *Receipt, b *Header, c uint) {

	//fmt.Println(a, b, c)
	fmt.Println("测试")
}

func (self *worker) GetConStatus() (string, uint64) {
	self.conviewmux.Lock()
	defer self.conviewmux.Unlock()
	return self.leader, self.view
}

func (self *worker) GetSelfNodeID() string {
	self.nidmux.RLock()
	defer self.nidmux.RUnlock()
	return self.nodeID
}

func (self *worker) Tostring(msg interface{}) string {
	//enc, err := rlp.EncodeToBytes(&msg)
	b, err := json.Marshal(msg)
	if err != nil {
		return ""
	}
	return string(b)
}

//对整个消息进行压缩，使用DeCompressToMsg解压缩，一般在handler.go中对应使用
func (self *worker) CompressAndTostring(msg interface{}) string {
	//enc, err := rlp.EncodeToBytes(&msg)
	b, err := json.Marshal(msg)
	if err != nil {
		return ""
	}
	cdata, err := dencom.Compress(b)
	if err != nil {
		return ""
	}
	return string(cdata)
}

func DeCompressToMsg(val string, conevent interface{}) error {
	//enc, err := rlp.EncodeToBytes(&msg)
	msg_byte, err := dencom.DeCompress([]byte(val))
	if err != nil {
		return err
	}

	err = json.Unmarshal(msg_byte, conevent)
	if err != nil {
		glog.V(logger.Info).Infoln("Unmarshal err %d", err)
		return err
	}
	return nil
}

func (self *worker) Ready() bool {
	self.rmux.Lock()
	defer self.rmux.Unlock()
	return self.ready
}

func (self *worker) primary() string {
	lid, _ := self.GetConStatus()
	return lid
}

//~~~~~~~~~~~~~~~~~~~~~~发起共识，发送request~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
//改：block替换为上面结构体
type Raaaa struct {
	Block     *Block
	Blockbyte []byte
}

type Reply struct {
	Error error
}

func (self *worker) SendRequest(request *Raaaa) { //block /*改： *BlockDM.Block*/ *Block, blockByte /*, txExec*/ []byte) {
	block := request.Block
	logms := LogMsg{
		Sender:  self.GetSelfNodeID(),
		Time:    time.Now(),
		Content: self.Tostring(block),
		Other:   "发送的区块",
	}
	Writelog("LogInfo/sendlog.txt", logms)
	//fmt.Println("发送的block为"+self.Tostring(block)+"jjj", time.Now().UTC())
	blockByte := request.Blockbyte
	_, cview := self.GetConStatus()
	self.LastReqID = self.LastReqID + 1

	//改：bh := (*block.Number()).Uint64()
	bh := (*block).Number
	//改：cBH := self.chain.CurrentBlock().NumberU64() // 获取区块链当前的合法高度
	cBH := self.chain.CurrentBlock

	if bh <= cBH {
		//如果小则返回，作为挖块节点，已经存储了pvb通过超时删除，可开启新一轮共识
		//如果因为新区块上链，pvb被删，那各类变量也会还原，可开启新一轮共识
		return
	}

	//改：
	req := Request{
		Realblock:    block,
		ReqID:        self.LastReqID, /*self.chain.CurrentBlock().NumberU64()+1,*/ //FIXME
		Sender:       self.GetSelfNodeID(),
		View:         cview,
		Timestamp:    time.Now(),
		BlockHeight:  (*block).Number,
		BlockHash:    block.Hash(),
		BlockCreator: common.HexToAddress(self.GetSelfNodeID()),
		Block:        blockByte,
		//realblock:    block,
		//直接传block过去，不使用上面的byte数组进行解码

		//TxsExeSeqs:   txExec,
		// Sig 暂时不写
	}

	//fmt.Println("7777" + self.Tostring(req.Realblock) + "8888888")
	cmsg := ConsensusMsg{
		Msg:       self.Tostring(req), //编码并压缩
		Timestamp: time.Now(),
		Type:      "PBFTRequest",
		Sender:    self.GetSelfNodeID(),
	}
	logmsg := LogMsg{
		Sender:  self.GetSelfNodeID(),
		Time:    time.Now(),
		Content: self.Tostring(cmsg),
		Other:   "发送request消息",
	}
	Writelog("LogInfo/sendlog.txt", logmsg)
	//fmt.Println(cmsg)
	//记录压缩用时
	/*可以注释吧应该
	if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
		//strings.Contains(s1,s2)如果s1字符串包含s2返回true
		digest := hashReq(&req)
		glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.Request.preHandleTime,", digest,
			sc.SepChar, time.Since(req.Timestamp).Nanoseconds())
	}
	*/
	//fmt.Println("a1发出去的block", req.Realblock)
	time.Sleep(time.Millisecond * 1000)
	go self.RecvRequest(&req)
	go self.PbftMux.Post(cmsg)

}

//~~~~~~~~~~~~~~~~~~~~~~事件处理函数~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

/*
** recvRequest 响应 Request 事件处理 新流程
** 判断lastReqId是否小于收到的req，如果小，设置成收到的reqid
** 判断request是否已存在
** 不接受old request，icorrect request
** 不接受区块高度小于当前区块 <=?
** 存储 request
** 如果view正确，发送preprepare消息
 */

func (instance *worker) RecvRequest(req *Request) error {

	var temp = LogMsg2{
		From:    req.Sender,
		Time:    time.Now(),
		Content: instance.Tostring(req),
		Other:   "收到的request消息",
	}
	Writelog("LogInfo/recivlog.txt", temp)
	//fmt.Println("a1收到的block", req.Realblock)
	//fmt.Println(instance.GetSelfNodeID(), "收到的req来自", req.Sender)
	/*应该可以注释吧
	if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
		t0 := time.Now()
		digest := hashReq(req)
		glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.RecvRequest,", digest, sc.SepChar,
			req.Sender[:len(req.Sender)/16], sc.SepChar, req.Timestamp.UnixNano(), sc.SepChar,
			t0.UnixNano(), sc.SepChar, req.BlockHeight, sc.SepChar, req.BlockHash.Hex())
	}*/
	if instance.LastReqID < req.ReqID { //如果lastRequest 小于当前req，更新为当前req
		instance.LastReqID = req.ReqID
	}

	if !instance.Ready() {
		glog.V(logger.Info).Infoln("ignore request, for being seletionleader  A11")
		return errors.New("being seletionleader A12")
	}
	// 不接受old request，icorrect request
	cleader, cview := instance.GetConStatus() //获取当前的leader和view
	if req.Sender != cleader || req.View < cview {
		glog.V(logger.Info).Infoln("req has wrong leader or view with bn  A14", req.Sender[:len(req.Sender)/16], req.View, req.BlockHeight)
		return errors.New("req has wrong leader or view  A15")
	}

	digest := newhashReq(req) // 判断request是否已存在。使用新的hash
	_, ok := instance.reqStore.Load(digest)
	if ok {
		glog.V(logger.Info).Infoln("req has already exists reqID, bn  A16", req.ReqID, req.BlockHeight)
		return errors.New("req has already exists    A17")
	}

	//cBH := instance.chain.CurrentBlock().NumberU64() // 获取区块链当前的合法高度
	cBH := instance.chain.CurrentBlock // 获取区块链当前的合法高度
	if req.BlockHeight <= cBH {
		glog.V(logger.Info).Infoln("A18 req block already onchain, bn %d, sender %d", req.BlockHeight, req.Sender[:len(req.Sender)/16])
		return errors.New("req block already onchain  A19")
	}

	//glog.V(logger.Detail).Infoln("Recv Request, view, bn", req.View, req.BlockHeight)

	instance.reqStore.Store(digest, req) // put in reqStore

	// TODO 当前 view 正确
	//_, cview = instance.GetConStatus()
	//if instance.inV(cview) {
	if instance.inV(req.View) {
		// preprepare消息
		preprep := PrePrepare{
			Sender:    instance.GetSelfNodeID(),
			View:      req.View,
			Timestamp: time.Now(),

			ReqID:         req.ReqID,
			RequestDigest: digest,
			Request:       req,
			// FIXME Sig 暂时不做处理
		}

		cmsg := ConsensusMsg{
			Msg:       instance.Tostring(preprep),
			Timestamp: time.Now(),
			Type:      "PBFTPrePrepare",
			Sender:    instance.GetSelfNodeID(),
		}

		//fmt.Println(cmsg)
		/*改：使用直接传过来的block
		block, err := BlockDM.DecodeRlp(req.Block)
		if err != nil {
			return err
		}*/
		block := req.Realblock
		instance.PutPVBAndBeats(block) // 将block放入待认证队列 		// record consensus time
		//fmt.Println("kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk")
		instance.putPrePrepareInCert(req.View, req.ReqID, preprep) // 将prepre放入cert
		//		instance.SetActiveView(false)                              // 将activeview设置为false，注别忘了设置为true

		//fmt.Println("mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm")
		//记录消息构建后与发送前的处理时间
		/*应该可以注释
		if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
			glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.PrePrepare.preHandleTime,", preprep.RequestDigest,
				sc.SepChar, time.Since(preprep.Timestamp).Nanoseconds())
		}*/
		//fmt.Println(preprep)
		//fmt.Println("lalalalallalalal")
		time.Sleep(time.Second * 5)
		go instance.RecvPrePrepare(&preprep)

		//fmt.Println("wowowowowowowowowowo")
		var temp2 = LogMsg{
			Sender:  instance.GetSelfNodeID(),
			Time:    time.Now(),
			Content: instance.Tostring(cmsg),
			Other:   "发送preprepare消息",
		}
		Writelog("LogInfo/sendlog.txt", temp2)
		go instance.PbftMux.Post(cmsg)

		return nil

	}

	return nil // 返回空
}

/*
** recvPrePrepare 响应 PrePrepare 事件处理
** 判断 PrePrepare 的发送节点是否为主节点
** 判断 view 是否正确
** 判断 PrePrepare 中的 request 的 transaction 是否在交易池中存在
** 发送 Prepare
** 存储 PrePrepare
 */
func (instance *worker) RecvPrePrepare(preprep *PrePrepare) (err error) {
	//FIXME catch panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("catch panic: %v ", r)
			glog.V(logger.Info).Info(err)

		}
	}()
	var temp = LogMsg2{
		From:    preprep.Sender,
		Time:    time.Now(),
		Content: instance.Tostring(preprep),
		Other:   "收到的preprepare消息",
	}
	Writelog("LogInfo/recivlog.txt", temp)
	//fmt.Println(instance.GetSelfNodeID(), "收到的prepre来自", preprep.Sender)
	/*应该可以注释吧
	if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
		glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.RecvPrePrepare,", preprep.RequestDigest, sc.SepChar,
			preprep.Sender[:len(preprep.Sender)/16], sc.SepChar, preprep.Timestamp.UnixNano(), sc.SepChar, time.Now().UnixNano())
	}*/
	instance.SyncView(preprep.View) //同步view FIXME 不应该每次都同步view
	//view出问题会广播
	//FIXME 有一个节点，会因为view获取不到，报view不同的错误，是不是缺一个获取当前view的机制
	cleader, view := instance.GetConStatus()
	if !instance.inV(preprep.View) {
		glog.V(logger.Info).Infoln("A20  Pre-pre view diff: prepre.View %d, expect.View %d, reqid %d", preprep.View, view, instance.primary()[:len(instance.primary())/16], preprep.ReqID)
		return nil
	}

	//if !instance.JudgeSync(preprep.Request.BlockHeight) { // 判断是否需要强制更新区块链，即从其他节点获取最新
	//	// thing must do
	//}
	fmt.Println("不需要更新区块链A1 preprepare函数")

	if !instance.Ready() {
		glog.V(logger.Info).Infoln("ignore prePrepare, for being seletionleader A21")
		return errors.New("being seletionleader  A22")
	}
	_, ok := instance.reqStore.Load(preprep.RequestDigest) //  判断Request是否已经存在
	if !ok {
		if preprep.Request.Sender != cleader || preprep.Request.View < view {
			glog.V(logger.Info).Infoln("A23 req has wrong leader or view with bn", preprep.Request.Sender[:len(preprep.Request.Sender)/16], preprep.Request.View, preprep.Request.BlockHeight)
			return errors.New("req has wrong leader or view A24")
		}
		if instance.LastReqID < preprep.ReqID { //如果lastRequest 小于当前req，更新为当前req，防止在没收到request的情况下不执行该过程
			instance.LastReqID = preprep.ReqID
		}
		//FIXME 保存下面这句，会导致RecvResquest执行没有效果，如果不执行，会导致下面的没办法执行
		instance.reqStore.Store(preprep.RequestDigest, preprep.Request)
		//go instance.RecvRequest(preprep.Request)                        // 触发req执行
	}

	cBH := instance.chain.CurrentBlock //().NumberU64() // 获取区块链当前的合法高度
	if preprep.Request.BlockHeight <= cBH {
		return errors.New("preprep.Request block already onchain")
	}

	// 获取 preprepare 对应的 cert
	//cert是个指针，在使用cert过程中可能变成空，变成空，说明消息被处理了，不需要继续执行了
	// 可以直接返回，所以如果发生panic情况，catch后返回err，不影响后面的运行
	cert := instance.getCert(preprep.View, preprep.Request.ReqID)

	//处理过不再处理
	if instance.prePrepared(preprep.RequestDigest, preprep.View, preprep.ReqID) && cert.sentPrepare {
		return nil
	}

	if cert.prePrepare != nil && cert.prePrepare.RequestDigest != preprep.RequestDigest {
		glog.V(logger.Info).Infoln("A25 Pre-prepare found for same view/seqNo but different digest: received %s, stored %s", preprep.RequestDigest, cert.prePrepare.RequestDigest)
	} else {
		instance.certMux.Lock()
		cert.prePrepare = preprep // track and store preprepare
		cert.IsPrepre = true      // 接收到 preprepare 消息
		instance.certMux.Unlock()
		if instance.prepared(preprep.RequestDigest, preprep.View, preprep.Request.ReqID) && !cert.sentCommit { // 即收到足够到prepared消息，且未发送Prepare
			glog.V(logger.Info).Infoln("A26  recv preprepare again ... ")
			instance.certMux.RLock()
			if len(cert.prepare) > 0 { //应该是为了加固，没加锁作判断，会因时间差导致执行时已清空
				go instance.RecvPrepare_notTransfer(&cert.prepare[0]) //为了防止preprep延迟导致，没有commit
			}
			instance.certMux.RUnlock()
		}
	}

	// 应该是为了加强防范，防止前面走到recvCommit，结束共识，清空消息
	if !instance.prePrepared(preprep.RequestDigest, preprep.View, preprep.ReqID) {
		glog.V(logger.Detail).Infoln("A27  recv preprep again: preprep.View %d,reqid %d", preprep.View, preprep.ReqID)
		return nil
	}

	_, ok = instance.reqStore.Load(preprep.RequestDigest)
	// Store the request if, for whatever reason, haven't received it from an earlier broadcast.
	if !ok {
		digest := newhashReq(preprep.Request)
		if digest != preprep.RequestDigest {
			glog.V(logger.Info).Infoln("A28  Pre-prepare request and request digest do not match: request %s, digest %s",
				digest, preprep.RequestDigest)
			return nil
		}
		// 存储所有request
		instance.reqStore.Store(digest, preprep.Request)
	}

	var block *Block
	if block = instance.GetBlockFromPossibleValidBlock(preprep.Request.BlockHeight, preprep.Request.BlockHash); block == nil {
		glog.V(logger.Debug).Infoln("A29 preprepare no pvb", preprep.Request.BlockHeight)

		/*改：不使用解码方法
		var err error
		block, err = BlockDM.DecodeRlp(preprep.Request.Block)
		if err != nil {
			return err
		}*/
		block = preprep.Request.Realblock
		instance.PutPVBAndBeats(block) // 将block放入待认证队列
	}

	if instance.prePrepared(preprep.RequestDigest, preprep.View, preprep.ReqID) && !cert.sentPrepare {
		if prepreflag == 0 {
			FormatOutput(instance.nodeID, instance.nodeID, "达到preprepare阶段")
			prepreflag = 1
		}
		/*注释掉原来的交易获取，使用新改的进行替换
		_, objects_ExeSeqs, dependency_TX, err := core.TxDependency(block.Transactions(), instance.pendingstate.GetStateCache())
		if err != nil {
			glog.V(logger.Info).Info("make txDependency failed, bn: ", block.Number())
			return err
		}
		*/
		_, objects_ExeSeqs, dependency_TX := TxDependency(block.Transactions())
		//txsExe := &core.TxsExeSequences{dependency_TX, objects_ExeSeqs}
		txsExe := &TxsExeSequences{objects_ExeSeqs, dependency_TX}

		/*
			txsExe_byte, err := txsExe.EncodeRlp()
			if err != nil {
				glog.V(logger.Info).Info("encode txExe err", err)
				return err
			}
		*/

		prep := Prepare{
			Sender:         instance.GetSelfNodeID(),
			View:           preprep.View,
			Timestamp:      time.Now(),
			ReqID:          preprep.ReqID,
			RequestDigest:  preprep.RequestDigest,
			TxsExeSeqs:     []byte(instance.Tostring(txsExe)),
			RealTxsExeSeqs: txsExe,
		}

		cmsg := ConsensusMsg{
			Msg:       instance.Tostring(prep), //有交易依赖图，可能也会较大，进行压缩
			Timestamp: time.Now(),
			Type:      "PBFTPrepare",
			Sender:    instance.GetSelfNodeID(),
		}

		//fmt.Println(cmsg)
		instance.certMux.Lock()
		cert.sentPrepare = true
		instance.certMux.Unlock()

		/*这部分可以注释掉我觉得
		if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
			glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.Prepare.preHandleTime,", prep.RequestDigest,
				sc.SepChar, time.Since(prep.Timestamp).Nanoseconds())
		}
		*/
		time.Sleep(time.Second * 5)

		go instance.PbftMux.Post(cmsg)
		var temp2 = LogMsg{
			Sender:  instance.GetSelfNodeID(),
			Time:    time.Now(),
			Content: instance.Tostring(cmsg),
			Other:   "发送prepare消息",
		}
		Writelog("LogInfo/sendlog.txt", temp2)
		go instance.RecvPrepare(&prep)

		//存储转发
		//上面不仅要发prep，现在还要给其他节点发preprep
		instance.sendConMsg_transfer1(preprep, "PBFTPrePrepare")

		return nil
	}

	return nil
}

/*
** recvPrepare 响应 Prepare 事件处理
** 判断 Prepare 的数量是否足够
** 判断 view 是否正确
** 判断 PrePrepare 中的 SequenceNumber 是否存在
** 发送 Commit
** 存储 prepare
 */
func (instance *worker) RecvPrepare(prep *Prepare) (err error) {
	//防止cert获取后，使用时为空
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("catch panic: %v ", r)
			glog.V(logger.Info).Info(err)

		}
	}()
	var temp = LogMsg2{
		From:    prep.Sender,
		Time:    time.Now(),
		Content: instance.Tostring(prep),
		Other:   "收到的prepare消息",
	}
	Writelog("LogInfo/recivlog.txt", temp)
	//fmt.Println(instance.GetSelfNodeID(), "收到的prepare来自", prep.Sender)
	/*改：
	if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
		glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.RecvPrepare,", prep.RequestDigest, sc.SepChar,
			prep.Sender[:len(prep.Sender)/16], sc.SepChar, prep.Timestamp.UnixNano(), sc.SepChar, time.Now().UnixNano())
	}
	*/
	if !instance.inV(prep.View) {
		return errors.New("prepare view is not correct")
	}

	if !instance.Ready() {
		glog.V(logger.Info).Infoln(" A30  ignore prepare, for being seletionleader")
		return errors.New("A31  being seletionleader")
	}
	//  判断Prepare是否已存在
	cert := instance.getCert(prep.View, prep.ReqID)
	for _, prePrep := range cert.prepare {
		if prePrep.Sender == prep.Sender { //sender相等，说明该sender发送的消息已经处理过了
			glog.V(logger.Debug).Infoln("A32  Ignoring duplicate prepare view %d reqID %d from %d", prep.View, prep.ReqID, prep.Sender[:len(prep.Sender)/16])
			return errors.New("A33  duplicate prepare msg  A1") //FIXME 存储转发时需要，否则需注释掉
		}
	}
	// 从reqStore中获取block
	req := instance.GetRequestFromStore(prep.RequestDigest)
	if req == nil {
		return errors.New("A34  do not have req")
	}
	//FIXME 删掉否
	cBH := instance.chain.CurrentBlock //().NumberU64() // 获取区块链当前的合法高度
	reqHeight := req.BlockHeight
	if reqHeight <= cBH {
		return errors.New("A35  preprep.Request block already onchain")
	}

	//如果已经发送过该区块的commit消息，直接返回
	//在这里额外记录的原因是，cert中存储的sentCommit标志，在checkpoint时会被清空
	//导致延迟的过期的commit消息被再次消费，如果checkpoint是因为消极观点，会走到这里
	reqHash := req.BlockHash
	commitMux.RLock()
	_, ok := commitStore[reqHeight]
	commitMux.RUnlock()
	if ok {
		//return nil
		return errors.New("A36  block commit have been sent a1.1")
	}

	//if !instance.JudgeSync(reqHeight) { //  根据Request的blockHeight进行同步
	//	// thing must do
	//}
	fmt.Println("不需要更新区块链A1 PREPARE函数")

	// 达到阈值后便不再执行
	//FIXME 为什么是！cert.sentCommit，为什么是非
	if instance.prepared(prep.RequestDigest, prep.View, prep.ReqID) && cert.sentCommit {
		return errors.New("A37  has already prepared")
	}

	instance.certMux.Lock()
	cert.prepare = append(cert.prepare, *prep) // add prepare
	instance.certMux.Unlock()
	if instance.prepared(prep.RequestDigest, prep.View, prep.ReqID) && !cert.sentCommit { // up to threshold
		if preflag == 0 {
			FormatOutput(instance.nodeID, instance.nodeID, "达到prepare阶段")
			preflag = 1
		}
		// 模拟执行区块 FIXME validate并没有在执行区块

		if reqHeight <= instance.chain.CurrentBlock {
			return errors.New("commit have sent")
		}
		block := instance.GetBlockFromPossibleValidBlock(req.BlockHeight, req.BlockHash)
		if block == nil {

			/*
				var err error
				block, err = BlockDM.DecodeRlp(req.Block)
				if err != nil {
					return err
				}
				glog.V(logger.Info).Info("recvPrepare pvb no such block")
				//instance.PutBlockOnPossibleValidBlock(block) // 将block放入待认证队列
			*/
			block = req.Realblock
		}

		/*改：使用自己的方法进行验证
		res := instance.pendingstate.Validator().ValidateBlock(block) // 只验证不写入,直接调用core/blockvalidate
		opnion := res == nil
		*/
		opnion := is_valid(block)

		if !opnion {
			glog.V(logger.Info).Infoln("----validate err")
		} else {

			//改：获取交易,获取通过共识的交易，若获取不到，则吧opnion设为false，虽然返回的交易没有用到，但是与要验证区块里面的交易是否达到共识
			t0 := time.Now()
			_, err := instance.getConsensusTxExes(prep.RequestDigest, prep.View, prep.ReqID)
			if time.Since(t0).Milliseconds() > 10 {
				glog.V(logger.Info).Info("======getConsensusTxExes time:", time.Since(t0))
			}

			if err != nil {
				glog.V(logger.Info).Infoln("getConsensusTxExes err:", err)
				opnion = false
			}

			/*
				//TODO 校验交易依赖图//是不是可以根据sender，让主节点不再校验
				_, objects_ExeSeqs, dependency_TX := core.TxDependency(block.Transactions(), instance.pendingstate.GetStateCache())
				txsExe := &core.TxsExeSequences{dependency_TX, objects_ExeSeqs}
				txsExe_byte, err := txsExe.EncodeRlp()
				if err != nil {
					glog.V(logger.Info).Infoln("----validate err", res)
					opnion = false
				} else {
					txsExe_msg := req.TxsExeSeqs
					hash_self := crypto.Keccak256Hash(txsExe_byte).Hex()
					hash_msg := crypto.Keccak256Hash(txsExe_msg).Hex()
					if hash_self != hash_msg {
						glog.V(logger.Info).Infoln("----txdepen not consensus", hash_self, hash_msg)
						opnion = false
					}
				}*/
		}
		vr := ValidResultForBlock{
			Sender:       instance.GetSelfNodeID(),
			BlockHeight:  req.BlockHeight,
			BlockHash:    req.BlockHash,
			BlockCreator: req.BlockCreator,
			Opinion:      opnion,
			Sign:         "",
			//TODO	sig
		}

		/*
			err := vr.Sign(instance.PNService.NodePrk)
			if err != nil {
				return errors.New("VR Sign Err")
			}
		*/
		//私钥加密使用签名替换
		err := instance.vrsign(&vr)
		if !err {
			return errors.New("VR Sign Err")
		}

		commit := Commit{
			Sender:        instance.GetSelfNodeID(),
			View:          prep.View,
			Timestamp:     time.Now(),
			ReqID:         prep.ReqID,
			RequestDigest: prep.RequestDigest,
			VR:            vr,
		}

		//避免并发，存在时间间隔，再次读取
		commitMux.Lock()
		_, ok = commitStore[reqHeight]
		if !ok {
			commitStore[reqHeight] = reqHash
		}
		for bh, _ := range commitStore {
			if bh < instance.chain.CurrentBlock { //().NumberU64() {
				delete(commitStore, bh)
			}
		}
		commitMux.Unlock()
		if ok {
			return errors.New("block commit have been sent  a1.2")
		}

		cmsg := ConsensusMsg{
			Msg:       instance.Tostring(commit),
			Timestamp: time.Now(),
			Type:      "PBFTCommit",
			Sender:    instance.GetSelfNodeID(),
		}
		//fmt.Println(cmsg)
		cert.sentCommit = true
		go instance.PbftMux.Post(cmsg)
		var temp2 = LogMsg{
			Sender:  instance.GetSelfNodeID(),
			Time:    time.Now(),
			Content: instance.Tostring(cmsg),
			Other:   "发送commit消息",
		}
		Writelog("LogInfo/sendlog.txt", temp2)
		time.Sleep(time.Second * 5)
		go instance.RecvCommit(&commit)

		//conn, errr := rpc.DialHTTP("tcp", "127.0.0.1:8094")
		//if errr != nil {
		//	log.Fatalln("dailing error: ", errr)
		//}
		//divCall := conn.Go("Worker.RecvCommit", &commit, nil, nil)
		//replyCall := <-divCall.Done // will be equal to divCall
		//if replyCall.Error != nil {
		//	log.Fatal("arith error:", replyCall.Error)
		//}
		//glog.V(logger.Info).Infoln("Send commit for view %d, reqID %d, blockNum %d, blockHash %d", prep.View, prep.ReqID, reqHeight, reqHash.Hex()[:8])
	}
	//存储转发
	instance.sendConMsg_transfer2(prep, "PBFTPrepare")

	//conn, errr := rpc.DialHTTP("tcp", "127.0.0.1:8094")
	//if errr != nil {
	//	log.Fatalln("dailing error: ", err)
	//}
	//divCall := conn.Go("Worker.RecvPrepare", prep, nil, nil)
	//replyCall := <-divCall.Done // will be equal to divCall
	//if replyCall.Error != nil {
	//	log.Fatal("arith error:", replyCall.Error)
	//}

	return nil
}

//在收到prepare时，消息存储后，可能会因为未收到preprepare,导致没有达到prepared，不会继续后面的处理
//增加相应的补偿机制，在收到preprepare时，会再次触发recvPrepare，增加共识效率和成功率
//由于正常的recvPrepare会进行判重，如果直接调用原来的recvPrepare，会在判重环节直接返回，机制无效
//新增如下方法，在再次触发recvPrepare时，调用下面的方法
//如果certSore中没有，则存储，且转发，如果有，则继续，但是不转发
func (instance *worker) RecvPrepare_notTransfer(prep *Prepare) (err error) {
	glog.V(logger.Info).Info("=====RecvPrepare_notTransfer=====")
	//防止cert获取后，使用时为空
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("catch panic: %v ", r)
			glog.V(logger.Info).Info(err)

		}
	}()
	var temp = LogMsg2{
		From:    prep.Sender,
		Time:    time.Now(),
		Content: instance.Tostring(prep),
		Other:   "收到的prepare消息",
	}
	Writelog("LogInfo/recivlog.txt", temp)
	//fmt.Println(instance.GetSelfNodeID(), "跳级收到prepare,未发送commit。来自", prep.Sender)
	/*
		if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
			glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.RecvPrepare,", prep.RequestDigest, sc.SepChar,
				prep.Sender[:len(prep.Sender)/16], sc.SepChar, prep.Timestamp.UnixNano(), sc.SepChar, time.Now().UnixNano())
		}
	*/

	if !instance.inV(prep.View) {
		return errors.New("prepare view is not correct")
	}
	if !instance.Ready() {
		glog.V(logger.Info).Infoln("ignore transferPrepare, for being seletionleader")
		return errors.New("being seletionleader")
	}
	//  判断Prepare是否已存在
	var recved = false
	cert := instance.getCert(prep.View, prep.ReqID)
	for _, prePrep := range cert.prepare {
		if prePrep.Sender == prep.Sender { //sender相等，说明该sender发送的消息已经处理过了
			recved = true //自己不会发给自己，所以若收到的preprep是自己发的，那肯定是其他节点转发的自己的又发回来了
			//那就证明自己已经向别的节点发送过prepare了，不用再发了，只需发commit即可
			glog.V(logger.Info).Infoln("Ignoring duplicate prepare view %d reqID %d from %d", prep.View, prep.ReqID, prep.Sender[:len(prep.Sender)/16])
			//return errors.New("duplicate prepare msg") //FIXME 存储转发时需要，否则需注释掉
		}
	}
	// 从reqStore中获取block
	req := instance.GetRequestFromStore(prep.RequestDigest)
	if req == nil {
		return errors.New("do not have req")
	}
	//FIXME 删掉否
	cBH := instance.chain.CurrentBlock //().NumberU64() // 获取区块链当前的合法高度
	reqHeight := req.BlockHeight
	if reqHeight <= cBH {
		return errors.New("preprep.Request block already onchain")
	}

	//如果已经发送过该区块的commit消息，直接返回
	//在这里额外记录的原因是，cert中存储的sentCommit标志，在checkpoint时会被清空
	//导致延迟的过期的commit消息被再次消费，如果checkpoint是因为消极观点，会走到这里
	reqHash := req.BlockHash
	commitMux.RLock()
	_, ok := commitStore[reqHeight]
	commitMux.RUnlock()
	if ok {
		return errors.New("block commit have been sent a1.3")
	}

	//if !instance.JudgeSync(reqHeight) { //  根据Request的blockHeight进行同步
	//	// thing must do
	//}
	fmt.Println("不需要更新区块链A1 准备2函数")

	// 达到阈值后便不再执行
	//FIXME 为什么是！cert.sentCommit，为什么是非
	if instance.prepared(prep.RequestDigest, prep.View, prep.ReqID) && cert.sentCommit {
		return errors.New("has already prepared")
	}

	instance.certMux.Lock()
	cert.prepare = append(cert.prepare, *prep) // add prepare
	instance.certMux.Unlock()
	if instance.prepared(prep.RequestDigest, prep.View, prep.ReqID) && !cert.sentCommit { // up to threshold

		// 模拟执行区块 FIXME validate并没有在执行区块

		if reqHeight <= instance.chain.CurrentBlock { //().NumberU64() {
			return errors.New("commit have sent")
		}
		block := instance.GetBlockFromPossibleValidBlock(req.BlockHeight, req.BlockHash)
		if block == nil {

			/*
				var err error
				block, err = BlockDM.DecodeRlp(req.Block)
				if err != nil {
					return err
				}
				glog.V(logger.Info).Info("RecvPrepare_notTransfer pvb no such block")
				//instance.PutBlockOnPossibleValidBlock(block) // 将block放入待认证队列
			*/
			block = req.Realblock

		}

		/*
			res := instance.pendingstate.Validator().ValidateBlock(block) // 只验证不写入,直接调用core/blockvalidate
			opnion := res == nil
		*/
		opnion := is_valid(block)
		if !opnion {
			glog.V(logger.Info).Infoln("----validate err")
		} else {

			t0 := time.Now()
			_, err := instance.getConsensusTxExes(prep.RequestDigest, prep.View, prep.ReqID)
			if time.Since(t0).Milliseconds() > 10 {
				glog.V(logger.Info).Info("======getConsensusTxExes time:", time.Since(t0))
			}

			if err != nil {
				glog.V(logger.Info).Infoln("getConsensusTxExes err:", err)
				opnion = false
			}

			/*
				//TODO 校验交易依赖图//是不是可以根据sender，让主节点不再校验
				_, objects_ExeSeqs, dependency_TX := core.TxDependency(block.Transactions(), instance.pendingstate.GetStateCache())
				txsExe := &core.TxsExeSequences{dependency_TX, objects_ExeSeqs}
				txsExe_byte, err := txsExe.EncodeRlp()
				if err != nil {
					glog.V(logger.Info).Infoln("----validate err", res)
					opnion = false
				} else {
					txsExe_msg := req.TxsExeSeqs
					hash_self := crypto.Keccak256Hash(txsExe_byte).Hex()
					hash_msg := crypto.Keccak256Hash(txsExe_msg).Hex()
					if hash_self != hash_msg {
						glog.V(logger.Info).Infoln("----txdepen not consensus", hash_self, hash_msg)
						opnion = false
					}
				}*/
		}
		vr := ValidResultForBlock{
			Sender:       instance.GetSelfNodeID(),
			BlockHeight:  req.BlockHeight,
			BlockHash:    req.BlockHash,
			BlockCreator: req.BlockCreator,
			Opinion:      opnion,
			Sign:         "",
			//TODO	sig
		}
		err := instance.vrsign(&vr)
		if !err {
			return errors.New("VR Sign Err")
		}

		commit := Commit{
			Sender:        instance.GetSelfNodeID(),
			View:          prep.View,
			Timestamp:     time.Now(),
			ReqID:         prep.ReqID,
			RequestDigest: prep.RequestDigest,
			VR:            vr,
		}

		//避免并发，存在时间间隔，再次读取
		commitMux.Lock()
		_, ok = commitStore[reqHeight]
		if !ok {
			commitStore[reqHeight] = reqHash
		}
		for bh, _ := range commitStore {
			if bh < instance.chain.CurrentBlock { //().NumberU64() {
				delete(commitStore, bh)
			}
		}
		commitMux.Unlock()
		if ok {
			return errors.New("block commit have been sent a1.4")
		}

		cmsg := ConsensusMsg{
			Msg:       instance.Tostring(commit),
			Timestamp: time.Now(),
			Type:      "PBFTCommit",
			Sender:    instance.GetSelfNodeID(),
		}
		//fmt.Println(cmsg)
		cert.sentCommit = true
		go instance.PbftMux.Post(cmsg)
		var temp2 = LogMsg{
			Sender:  instance.GetSelfNodeID(),
			Time:    time.Now(),
			Content: instance.Tostring(cmsg),
			Other:   "发送commit消息",
		}
		Writelog("LogInfo/sendlog.txt", temp2)
		time.Sleep(time.Second * 5)
		go instance.RecvCommit(&commit)

		//conn, errr := rpc.DialHTTP("tcp", "127.0.0.1:8094")
		//if errr != nil {
		//	log.Fatalln("dailing error: ", err)
		//}
		//divCall := conn.Go("Worker.RecvCommit", &commit, nil, nil)
		//replyCall := <-divCall.Done // will be equal to divCall
		//if replyCall.Error != nil {
		//	log.Fatal("arith error:", replyCall.Error)
		//}
		//glog.V(logger.Info).Infoln("Send commit for view %d, reqID %d, blockNum %d, blockHash %d", prep.View, prep.ReqID, reqHeight, reqHash.Hex()[:8])
	}
	//如果未收到，则存储转发
	if !recved {
		instance.sendConMsg_transfer2(prep, "PBFTPrepare")
	}
	return nil
}

/*
** recvCommit 响应 Commit 事件处理
** 判断 commit 的数量是否足够
** 判断 view 是否正确
** 判断 Commit 中的 SequenceNumber 是否存在
** 存储 Commit
** 写入 cspool 交易池
** 主节点启动挖矿，即构建区块
 */
func (instance *worker) RecvCommit(commit *Commit) (err error) {

	//FIXME catch panic
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("catch panic: %v ", r)
			glog.V(logger.Info).Info(err)
		}
	}()
	var temp = LogMsg2{
		From:    commit.Sender,
		Time:    time.Now(),
		Content: instance.Tostring(commit),
		Other:   "收到的commit消息",
	}
	Writelog("LogInfo/recivlog.txt", temp)
	//fmt.Println(instance.GetSelfNodeID(), "接受commit来自", commit.Sender)
	/*
		if strings.Contains(sc.GetMinerLog(), sc.Consensus) {
			glog.V(logger.Info).Infoln(sc.TestPrefix, "worker.RecvCommit,", commit.RequestDigest, sc.SepChar,
				commit.Sender[:len(commit.Sender)/16], sc.SepChar, commit.Timestamp.UnixNano(), sc.SepChar, time.Now().UnixNano())
		}
	*/

	if !instance.inV(commit.View) {
		glog.V(logger.Info).Infoln("===commit view is not correct", commit.VR.BlockHeight)
		return errors.New("commit view is not correct")
	}

	if !instance.Ready() {
		glog.V(logger.Info).Infoln("ignore commit, for being seletionleader")
		return errors.New("being seletionleader")
	}
	fmt.Println("测试", commit.VR.BlockHeight, instance.chain.CurrentBlock, commit.Sender)
	cBH := instance.chain.CurrentBlock //().NumberU64() // 获取区块链当前的合法高度
	if commit.VR.BlockHeight <= cBH {  //FIXME =
		return errors.New("preprep.Request block already onchain")
	}

	//if !instance.JudgeSync(commit.VR.BlockHeight) { // 根据request的blockHeight
	//	//thing must do
	//}
	fmt.Println("不需要更新区块链   A1提交函数")

	cert := instance.getCert(commit.View, commit.ReqID)
	for _, prevCommit := range cert.commit {
		if prevCommit.Sender == commit.Sender { // 存在重复的commit消息
			//收到的是自己的，应该就是别人转发的自己的又发回来了？？？
			glog.V(logger.Debug).Infoln("Ignoring duplicate commit from %d", commit.Sender[:len(commit.Sender)/16])
			return errors.New("duplicate commit msg") //FIXME 存储转法时需要，否则注释掉
		}
	}

	instance.certMux.Lock()
	cert.commit = append(cert.commit, *commit) // add commit
	instance.certMux.Unlock()
	glog.V(logger.Info).Infoln("Replica %d received commit from %d v %d req %d bNum %d",
		instance.GetSelfNodeID()[:len(instance.GetSelfNodeID())/16], commit.Sender[:len(commit.Sender)/16], commit.View, commit.ReqID, commit.VR.BlockHeight)

	if instance.committed(commit.RequestDigest, commit.View, commit.ReqID) { // up to committed
		if commitflag == 0 {
			FormatOutput(instance.nodeID, instance.nodeID, "达到commit阶段")
		}
		//fmt.Println("timi",time.Since(committed))
		votesRes := instance.GetValidResFromCommit(cert)
		glog.V(logger.Info).Infoln("=====Reach committed, bn %d len(commit) %d votesRes %d", commit.VR.BlockHeight, len(cert.commit), votesRes)

		//交易依赖图需要用到，区块获取可能也需要，先获取
		req := instance.GetRequestFromStore(commit.RequestDigest)
		if req == nil {
			glog.V(logger.Info).Infoln("=====cant get req by reqstore when commit", commit.VR.BlockHeight)
			return errors.New("cant get commit req")
		}

		if votesRes == possitive { // 若积极的观点达到阈值
			//  获取Block，先从possibleValideBlock中获取，若没有再从reqstore中获取
			block := instance.GetBlockFromPossibleValidBlock(commit.VR.BlockHeight, commit.VR.BlockHash)
			if block == nil {

				/*
					var err error
					block, err = BlockDM.DecodeRlp(req.Block)
					if err != nil {
						return err
					}
				*/
				block = req.Realblock
			}

			//如果没有错误，将block 及验证结果写入数据库
			//将共识的结果写入数据库，格式为kv，key为“block-区块的hash值-VRS”, value为所有的合法结果，即ValidResultForBlock
			// 持久化交易
			validResult := make(map[string]ValidResultForBlock)
			for _, ct := range cert.commit {
				if ct.VR.Opinion {
					validResult[ct.VR.Sender] = ct.VR
				}
			}

			/*
				vrs, err := VRS.Encode(validResult)
				if err != nil {
					glog.V(logger.Info).Infoln(" ===VRS.Encode(validResult) err=%d", err, commit.VR.BlockHeight)
					return errors.New("encode validResult Err")
				}
			*/
			//glog.V(logger.Info).Infoln("time of possitive->insertchain",time.Since(committbefore))
			//insertbefore := time.Now()

			//改：交易貌似不会用到,但是要获取，用下面的获取，
			//txExe_byte, _ := instance.getConsensusTxExes(commit.RequestDigest, commit.View, commit.ReqID)
			//txExe, err := core.DecodeRlp_TxsExec(txExe_byte)
			//if err != nil {
			//	glog.V(logger.Info).Infoln(" ===core.DecodeRlp_TxsExec(txexe_byte) err=%d", err, commit.VR.BlockHeight)
			//	return errors.New("decode txsExes err")
			//}

			_, err := instance.getConsensusTxExes(commit.RequestDigest, commit.View, commit.ReqID)
			if err != nil {
				glog.V(logger.Info).Infoln("getConsensusTxExes err:", err)
			}

			if !instance.Ready() {
				glog.V(logger.Info).Infoln("stop to insertblock, for being seletionleader")
				return errors.New("being seletionleader")
			}

			//改：插入到chain中，简单一点吧
			/*
				if _, err := instance.chain.InsertChainWithVRS(BlockDM.Blocks{block}, VRS.VRSSForStorage{vrs}, []*core.TxsExeSequences{txExe}); err != nil {
					glog.V(logger.Error).Infoln("insert chain err", err)
					glog.V(logger.Info).Infoln("=====len(block.txs)", len(block.Transactions()))
					for _, tx := range block.Transactions() {
						glog.V(logger.Info).Infoln("=====txhash, txop: ", tx.Hash().Hex(), tx.Op())
						break
					}
				}
			*/

			if commit.VR.BlockHeight > instance.chain.CurrentBlock {
				fmt.Println(commit.VR.BlockHeight)
				fmt.Println("区块高度before为：", instance.chain.CurrentBlock, commit.Sender)
				InsertChainWithVRS()
				Heightlock.Lock()
				if commit.VR.BlockHeight > instance.chain.CurrentBlock {
					instance.chain.CurrentBlock++
				}
				Heightlock.Unlock()
				fmt.Println("区块高度after为：", instance.chain.CurrentBlock, commit.Sender)
			} else {
				return nil
			}
			//fmt.Println("区块高度after为：", instance.chain.CurrentBlock, commit.Sender)

			time.Sleep(time.Second * 5)
			// TODO downloader或blockchain验证区块的规则也应修改，需要同时验证VRS
			go instance.mux.Post(Checkpoint{instance.GetSelfNodeID(), commit.VR.BlockHeight, commit.ReqID, commit.VR.BlockHash})
			var temp2 = LogMsg{
				Sender:  instance.GetSelfNodeID(),
				Time:    time.Now(),
				Content: instance.Tostring(Checkpoint{instance.GetSelfNodeID(), commit.VR.BlockHeight, commit.ReqID, commit.VR.BlockHash}),
				Other:   "发送checkpoint消息",
			}
			Writelog("LogInfo/sendlog.txt", temp2)

			instance.removeOldConMsg(commit.ReqID) //   删除相应的共识消息

			//改：无人接受？？？，先注释
			//go instance.mux.Post(NewMinedBlockEvent{Block: block, VRS: ""}) //TODO 发送消息，待定，看mining注释有，

			//glog.V(logger.Info).Infoln("pbft send core.ChainHeadEvent", block.NumberU64())
			//go instance.mux.Post(core.ChainHeadEvent{Block: block}) // 发送消息，待定，看mining注释有，
			//glog.V(logger.Info).Infoln("time of possitive handle",time.Since(committbefore))

		}

		if votesRes == negative { // 不会再达到积极
			time.Sleep(time.Second * 5)
			go instance.mux.Post(Checkpoint{instance.GetSelfNodeID(), commit.VR.BlockHeight, commit.ReqID, commit.VR.BlockHash})
			var temp2 = LogMsg{
				Sender:  instance.GetSelfNodeID(),
				Time:    time.Now(),
				Content: instance.Tostring(Checkpoint{instance.GetSelfNodeID(), commit.VR.BlockHeight, commit.ReqID, commit.VR.BlockHash}),
				Other:   "发送checkpoint消息",
			}
			Writelog("LogInfo/sendlog.txt", temp2)
			//instance.SendViewchange(instance.chain.CurrentBlock) //().NumberU64()) // 更换主节点
			lid, view := instance.GetConStatus()
			glog.V(logger.Info).Infof("negative: replica= %v Send Viewchange cleader= %v curview= %d bn= %d", instance.GetSelfNodeID()[:len(instance.GetSelfNodeID())/16], lid[:len(lid)/16], view, commit.VR.BlockHeight)

			instance.removeOldConMsg(commit.ReqID) //   删除相应的共识消息

		}

		//FIXME 用于存储转发
		//if votesRes != possitive { //如果是积极，直接同步区块即可，转发的必要性不大，暂时不转发
		//	instance.sendConMsg_transfer(commit, "PBFTCommit")
		//}

	}
	instance.sendConMsg_transfer3(commit, "PBFTCommit")
	return nil
}

//TODO RecvCheckpoint优化，checkpoint执行的内容待优化，触发checkpoint执行的点待优化
//主要是删除共识池消息，就是cert那些消息，因为共识通过了，不管共识通过与否，都要删除消息
// RecvCheckpoint
func (instance *worker) RecvCheckpoint( /*bn uint64, bhash common.Hash, reqID uint64*/ chkpt *Checkpoint) error {
	//time.Sleep(time.Millisecond *100)
	updateLeaseBeat() //如果因上链触发，属于正常情况，如果因错误区块更新，也没有问题，会发起重新选举，这个时间一定会更新的
	bn := chkpt.BlockHeight
	bhash := chkpt.BlockHash
	//reqID := chkpt.ReqID

	//defer chkptGroup.Done()
	defer func() {
		instance.setChkpt(false)
	}()
	//if instance.getChkpt() == false {
	//	return errors.New("instance.getChkpt()==false")
	//}
	var temp = LogMsg2{
		From:    chkpt.Sender,
		Time:    time.Now(),
		Content: instance.Tostring(chkpt),
		Other:   "收到的checkpoint消息",
	}
	Writelog("LogInfo/recivlog.txt", temp)
	//fmt.Println(instance.GetSelfNodeID(), "收到checkpoints来自", chkpt.Sender)
	if _, ok := checkpointStore.Load(bhash); ok {
		return errors.New("error")
	}

	updateCheckpointBH(bn)

	cbn := instance.chain.CurrentBlock //().NumberU64()
	if bn < cbn {
		return errors.New("error")
	}

	//checkpointStore.Store(bn, false)
	//for i := uint64(0); i < cbn; i++{ //FIXME bn 还是cbn bn会大于cbn吗？
	//	checkpointStore.Delete(i)
	//}

	glog.V(logger.Info).Infof("Received checkpoint BlockNumber=%d", bn)
	// viewchange 或 checkpoint时，activeView设置为false，即不允许共识交易
	//	instance.SetActiveView(false)

	if consensusCancel != nil {
		consensusCancel()
		//glog.V(logger.Info).Infoln("====cancel",instance.chain.CurrentBlock().Number())

	}

	//  删除possibleValidBlock中小于等于block.Height的区块
	instance.DeletePVBAndBeats(bn, bhash)

	//instance.removeOldConMsg(reqID) //   删除相应的共识消息
	//	instance.SetActiveView(true) //将active设置为true

	checkpointStore.Store(bhash, bn)
	checkpointStore.Range(func(k, v interface{}) bool {
		if v.(uint64) < cbn { //小于cbn 的将被前面的直接被拦截，可以删除记录
			checkpointStore.Delete(k)
		}
		return true
	})

	leader, view := instance.GetConStatus()
	glog.V(logger.Info).Infoln("current view is %d cleader is %d ", view, leader[:len(leader)/16])

	//instance.LeaseChange(bn) // lease check

	//改：只共识一次
	//if instance.eth.TxPool().ExistsTxs() { // resume consensus
	//	go instance.randomwait_StartConsensus(100)
	//}
	if false { // resume consensus
		go instance.randomwait_StartConsensus(100)
	}

	fmt.Println("奇怪", chkpt.BlockHeight, instance.chain.CurrentBlock)
	fmt.Println("共识结束")

	time.Sleep(time.Millisecond * 1000)
	return nil
}

// StartConsensus 开启共识流程
// TODO 使用context使得全局同一时刻有且仅有一个StartConsensus在执行
func (self *worker) StartConsensus() error {
	// 不允许并发执行，判断是否有其他的区块在进行共识，利用waitgroup
	// 然后共识完成后等待交易池再次发送消息给共识，或者直接判断交易池中是否有数据

	// 注：新版本的共识过程中，不再启动Agent去构建区块（挖矿），而由worker构建
	// 注：需要将与agent通信的地方封掉，例如self.push函数
	// 注：需要更改wait和commitNewwork函数的过程，将wait函数改造成区块共识通过后执行
	//    commitNewWork 不在需要执行

	// 注：在原有机制中，worker将构建区块的所需信息封装成work（即工作量）发送给agent，
	//     agent根据work构建区块，然后将构建好的区块以Result的数据结构发送给worker，
	//     worker收到Result之后（即wait函数），更新本地self.work并将区块发送给blockchain和txpool等
	//     故需要将原来与agent交互的过程全部封掉

	// 注： 新的共识流程整体如下：txpool发送TxPreEvent事件，worker接受到TxPreEvent事件后触发一下流程
	//     1. 判断当前是否以开启挖矿流程，若已开启则不执行，但需提供一个标志位（hasUnDealTxs boolean类型）用于表示是否还有未处理的交易
	//     2. worker调用StartConsensus开启共识过程，（注：一次有且仅能由一个共识进程开启，需要通过waitgroup或者context进行预防多个共识进行开启的情况）
	//     2.1 若当前有超过2/3的节点存活，且当前主节点为本节点，则构建区块block，否则不执行
	//     2.2 根据pendingState构建block，构建完成后将pendingstate重置为chain
	//     2.3 将block放入possibleValidBlock队列中
	//     2.4 将block封装成request发送出去
	//     2.5 当block接受到2/3的commit消息时，执行wait函数并将区块广播至txpool和blockchain，
	//         将pendingState更新，删除possileValidBlock中已经过时与已经共识过的区块
	//	   	   将共识的投票结果写入数据库
	//	   2.6 清空共识消息
	//     // 暂不考虑 2.7 对于主节点，若hasUnDealTxs为true，则开启新的一轮共识
	// 注：节点对于任何消息均需回复，无法是正确或错误的回复

	// 注：其他稳定性、鲁棒性措施 暂不实现
	//    1. 若某节点接受到共识信息时，由于当前账本不同步或者执行的慢的原因导致无法验证当前区块的问题，
	//       以此造成误将合法的区块认为错误的区块，由此导致共识难以正常进行
	//       解决方案： Solution.1.1 SendRequest时，请求中的区块的高度必须大于节点的账本高度
	//                              当请求中的区块高度过大时，节点向其周围节点同步区块
	//                              当同步完成时，即获取到Request需要的高度时，发送request的下一级消息
	//                              注：或者将更新区块阻塞，直到更新完成才可处理request

	//                 Solution.1.2 主节点的retry与request超时处理机制
	//                              当每一个request的处理完成设置一个时间阈值，即10s，
	//                              10s内没有收集到2/3的结果（正确或错误）便认为是超时
	//                              重新发起requst请求，request带有一个自增的标号
	//                              若一直发生retry，则需人工介入
	//
	//   2. 主节点监控与监管
	//      交易池中维护每个合法交易的上链时间，当超时，发起更换请求
	//      当接收到请求时需要判断是否存在超时交易
	//   3. 主节点更换
	//      lease正常更换
	//      当主节点构建了错误的区块时，重新选举的过程
	//   4. 保证选举过程是拜占庭容错的
	//      选举带有不同的reason，即时正常lease更换、超时、错误区块等情况，优先级？
	//   5. 交易超时问题
	//      没有最优解
	//
	// 思考：若采用dpos作为选举过程呢？
	//

	// 注： 去掉miner.start与stop等函数
	// 原则上必须按照线性序列校验区块

	// protocolManager         txpool             worker
	//      |                    |                  |
	//      |       --tx-->      |                  |
	//      |                    | ---TXPreEvent--> |
	//      |                    |                  |-consensus processing
	//      |                    |                  |
	//      |                    |   <--Block--     |
	//      |                    |                  |
	//      |                    |-delete txs       |
	//      |                    |                  |
	//      |                    |--CacheClearly--> |
	//      |                    |                  |-start new consensus processing
	//      |                    |                  |
	//      |                    |                  |
	//      |                    |                  |
	//      |                    |                  |
	//      |                    |                  |

	self.mu.Lock()
	defer self.mu.Unlock()
	self.currentMu.Lock()
	defer self.currentMu.Unlock()
	//glog.V(logger.Info).Infoln("=======startconsensus  ", self.chain.CurrentBlock().NumberU64()+1)

	//仅用于共识调试，用于构建包含一定数量交易的区块
	//如果不调用接口重新设置，不会进入if内部，不影响正常运行
	if self.txNum_startconsens > 0 {
		Txs, _ := self.eth.TxPool()
		count := len(Txs)
		//返回的是交易数组，数组长度就是交易池里面的交易个数

		//for _, txs := range Txs {
		//	count += len(txs)
		//}
		if count < self.txNum_startconsens {
			self.hasUnDealTxs = true // 用于可持续性共识与主节点异常检测
			return errors.New("error")
		}
	}

	if self.CanMining() {
		self.pendingstate = self.chain //.Copy() // 将pendingState重置
		//self.conPNode = self.PNService.GetPermissionedConNodes() // 获取所有许可的共识节点
		//self.N = lenOfSyncMap(self.conPNode)                     // 共识节点数量

		if consensusCtx == nil || consensusCtx.Err() != nil { //执行cancel后，err != nil
			//glog.V(logger.Info).Infoln("====init ctx",self.chain.CurrentBlock().Number())
			glog.V(logger.Info).Infoln("====1===mining bn, pnodes===  ", self.chain.CurrentBlock /*().NumberU64()*/ +1, self.N)
			consensusCtx, consensusCancel = context.WithCancel(context.Background())
		} else {
			glog.V(logger.Info).Infoln("====8======  ctx not nil, bn, beat, pb, ", self.chain.CurrentBlock /*().Number()*/, len(self.blockbeats), len(self.possibleValidBlock))
			return errors.New("error")
		}
		atomic.StoreInt32(&self.mining, 1)

		// 注: self.mining 与self.atwork 均为atomic,
		//    mining 用于表示是否正在构建区块
		//    atwork 用于表示是否正在执行本节点所构建的区块

		// TODO Header与Block的数据结构中去除Gas、Nonce相关的属性
		// 创建Header
		tstart := time.Now()
		parent := self.chain.ReturnCurrentBlock()
		tstamp := tstart.Unix()
		//按秒累加，在大量区块持续上链情况下，时间一直累加，会严重偏离真实情况，且越来越严重
		//if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		//	tstamp = parent.Time().Int64()
		//}

		num := int64(parent.Number) //().Int64()
		//header := HeaderDM.NewHeader()
		header := new(Header)
		//header.SetParentHash(parent.Hash())
		//header.SetNumber(big.NewInt(num + 1))
		//header.SetCoinbase(self.coinbase)
		//header.SetExtra(self.extra)
		//header.SetTime(big.NewInt(tstamp))
		//header.SetChainName(self.chainName)
		header.ParentHash = parent.Hash()
		header.Number = big.NewInt(num + 1)
		header.Coinbase = self.coinbase
		header.Extra = self.extra
		header.Time = big.NewInt(tstamp)
		header.ChainName = self.chainName

		previous := self.current //用于重置work
		//t00 := time.Now()
		err := self.makeCurrent(parent, header)
		//glog.V(logger.Info).Infoln("=====makeCurrent", time.Since(t00))
		if err != nil {
			glog.V(logger.Info).Infoln("Could not create new env for mining, retrying on next block.")
			//consensusCtx = nil
			if consensusCancel != nil {
				consensusCancel()
				//glog.V(logger.Info).Infoln("====cancel",self.chain.CurrentBlock().Number())
			}
			return errors.New("error")
		}
		// 获取要上链的交易
		//txs := TXDM.NewTransactionsByPriceAndNonce(self.eth.TxPool().Pending()) // 获取交易池中的所有合法交易

		//txs := make(map[common.Address]TXDM.Transactions,0)//FIXME 为了构建空区块
		//==============================

		/*
			multiVTxs := self.eth.TxPool().Pending() // 获取交易池中的所有合法交易
			txs := multiVSelectWithLimit(multiVTxs) //多版本选择
			t0 := time.Now()
			committedSet := TXDM.NewTransactionsByTypeAndNonce(txs)
			// 对committedSet中的所有交易执行commitTransaction
			work := self.current
			//t1 := time.Now()
			work.commitTransactions(self.mux, committedSet, self.pendingstate) // 注传入的是pendingState，（账本的深copy对象）
			glog.V(logger.Info).Infoln("commitTransactions time",len(committedSet.Txs),time.Since(t0))
		*/

		//=========================================

		//=================================================
		//		glog.V(logger.Info).Infoln("=====for debug=====")
		//t0 := time.Now()

		/*这里只需要获取交易就行
		var multiVTxs map[common.Address]TXDM.TransactionsWithTime
		reschan := make(chan int64)
		var wg sync.WaitGroup
		wg.Add(1)
		go func(reschan chan int64, wg *sync.WaitGroup) {
			for {
				select {
				case msg := <-reschan:
					switch msg {
					case core.Run:
						//glog.V(logger.Info).Info("---------")
						multiVTxs = self.eth.TxPool().PendingWithTime() // 获取交易池中的所有合法交易
						wg.Done()
						goto Loop
					case core.Wait:
						//glog.V(logger.Info).Info("---------")
					case core.Cancel:
						//glog.V(logger.Info).Info("---------")
						wg.Done()
						goto Loop
					}
				}
			}
		Loop:
		}(reschan, &wg)*/

		//glog.V(logger.Info).Infoln("=====PendingWithTime", time.Since(t0))

		//改
		//go self.eth.TxPool().TxPoolManager().PendingWithTime(num+1, reschan) // 获取交易池中的所有合法交易
		//wg.Wait()
		//go self.eth.TxPool().TxPoolManager().PendingWithTimeEnd()
		//go close(reschan)
		//t1 := time.Now()
		multiVTxs, multiVTxs2 := self.eth.TxPool()

		//选择一个区块可以容纳的交易数
		txs, txs2 := multiVSelectWithLimit2(multiVTxs, multiVTxs2) //多版本选择

		// 对committedSet中的所有交易执行commitTransaction
		//glog.V(logger.Info).Infoln("=======txSelect====", time.Since(t1))

		work := self.current
		//state := work.state
		//t10 := time.Now()
		stringte := "没用"
		committedSet, objects_ExeSeqs, dependency_TX := TxDependency(stringte, txs, txs2)

		fmt.Println("执行1.。。。。。。。。。。。。。。。。。。。。。。。。。。。")
		/*
			if err != nil {
				glog.V(logger.Info).Info("make txdependcy failed, bn: ", header.GetNumber())

				//consensusCtx = nil
				if consensusCancel != nil {
					consensusCancel()
					//glog.V(logger.Info).Infoln("====cancel",self.chain.CurrentBlock().Number())
				}
				return
			}*/

		//glog.V(logger.Info).Infoln("=====for debug=====finished txDependency")

		//glog.V(logger.Info).Infoln("=====txDependency All==",time.Since(t10))

		//t1 := time.Now()
		//提前byte化，防止执行时，指针被修改
		//txsExe := &core.TxsExeSequences{dependency_TX, objects_ExeSeqs}
		//txsExe_byte, err := txsExe.EncodeRlp()
		//if err != nil {
		//	glog.V(logger.Info).Infoln("rlp txsExe err :", err)
		//	if consensusCancel != nil {
		//		consensusCancel()
		//		//glog.V(logger.Info).Infoln("====cancel",self.chain.CurrentBlock().Number())
		//	}
		//	return
		//}

		work.commitTransactions_parallel(self.mux, committedSet, self.pendingstate, objects_ExeSeqs, dependency_TX) // 注传入的是pendingState，（账本的深copy对象）
		//glog.V(logger.Info).Infoln("=====commitTxs_parallel time==",len(committedSet),time.Since(t1))
		//glog.V(logger.Info).Infoln("=====len(tx)===All time==",len(committedSet),time.Since(t0))
		//glog.V(logger.Info).Infoln("=====for debug=====finished commitTransactions_parallel")

		//t2 := time.Now()
		//==========================================================
		var uncles = []*Header{header}

		//if header.GetNumber().Cmp(big.NewInt(5)) == 0 {
		//	work.state.CreateAccountByType(common.Address{}, &dencom.TransactionID{CurrentHash: common.Hash{}}, systemconfig.Contract)
		//}
		// 封装work

		//改：
		header.Root = string(num+1) + header.ParentHash
		fmt.Println("执行3.......................................")
		//header.SetRoot(work.state.IntermediateRoot(header.GetNumber(), header.GetParentHash()))

		//glog.V(logger.Info).Infoln("=====for debug=====finished header.SetRoot")

		//glog.V(logger.Info).Infoln("=====header.SetRoot==",time.Since(t2))

		//t3 := time.Now()
		var namaa = Receipt{
			String: "receipt",
		}

		var rece = []*Receipt{&namaa}
		work.receipts = rece
		for i, receipt := range work.receipts {
			ReSetReceipts(receipt, header, uint(i))
		}
		var blockk = new(Block)
		work.Block = blockk
		//fmt.Println("执行4.........................")
		work.Block.Receipt = work.receipts
		//fmt.Println("执行6.............")
		work.Block.Number = uint64(num + 1)
		var tran = Transaction{"dddd"}
		var txxs = []*Transaction{&tran}
		work.txs = txxs
		work.Block.Txs = work.txs
		//fmt.Println("zhixing8............................")
		work.Block.Uncles = uncles
		//fmt.Println("zhixing9..................................")
		work.Block.Header = header

		//fmt.Println("执行5............")
		//work.Block = BlockDM.NewBlock(header, work.txs, uncles, work.receipts)
		//glog.V(logger.Info).Infoln("=====newblock==",time.Since(t3))

		//t4 := time.Now()

		/*
			blockByte, err := work.Block.EncodeRlp()
			if err != nil {
				glog.V(logger.Info).Infoln("new block rlpEncodeErr")
				if consensusCancel != nil {
					consensusCancel()
					//glog.V(logger.Info).Infoln("====cancel",self.chain.CurrentBlock().Number())
				}
				return
			}*/
		blo := "随便"
		blockByte := []byte(blo)

		//glog.V(logger.Info).Infoln("=====block EncodeRlp==",time.Since(t4))

		self.PutPVBAndBeats(work.Block) // 将区块放入区块池
		//	fmt.Println("77777777777777")
		self.pendingstate = self.chain //.Copy() // 将pendingState重置
		self.current = previous        // 将worker重置

		//	fmt.Println("666666666666666666666666666666666")
		send := new(Raaaa)
		send.Block = work.Block
		send.Blockbyte = blockByte
		//	fmt.Println("执行2.。。。。。。。。。。。。。。。。。。。。。。。。。。。")
		fmt.Println(self.Tostring(send))
		self.SendRequest(send)             //work.Block, blockByte /*, txsExe_byte*/) // 以block为单位
		atomic.StoreInt32(&self.mining, 0) // 结束挖矿

		//glog.V(logger.Info).Infof("🔨 🔗  Mined %d blocks back: block #%v", miningLogAtDepth, inspectBlockNum)
		self.logLocalMinedBlocks(work, previous)

	} else {
		// 仍然对hasUnDealTxs进行管理
		self.hasUnDealTxs = true // 用于可持续性共识与主节点异常检测
	}

	//fmt.Println(a)
	//reply.Error = nil
	return nil
}

// expirationLoop is a loop that periodically iterates over all accounts with
// queued transactions and drop all that have been inactive for a prolonged amount
// of time.
func (self *worker) expirationLoop() {
	evict := time.NewTimer(evictionInterval)
	defer evict.Stop()
	for {
		select {
		case <-evict.C:

			isExpire := false
			glog.V(logger.Info).Infoln("check expritaiton b p", len(self.blockbeats), len(self.possibleValidBlock))
			self.pvbAndBeatsMux.Lock()
			for blockid, beat := range self.blockbeats {
				if time.Since(beat) > maxConsensusLifetime {
					//FIXME  有且仅当blockid所表示的区块高度大于当前高度时
					// isExpire才置为True
					bh, err := self.getPossibleBHByHash(blockid)
					if err == nil { //存在bh 才置为 true
						if bh == self.chain.CurrentBlock /*().NumberU64()*/ +1 {
							isExpire = true
						}
					}
					//if consensusCancel != nil {
					//	consensusCancel()
					//glog.V(logger.Info).Infoln("====cancel",self.chain.CurrentBlock().Number())
					//}

					self.deletePVBAndBeatsByHash(blockid) // reset

				}
			}
			self.pvbAndBeatsMux.Unlock()
			if isExpire {
				if consensusCancel != nil {
					consensusCancel()
					//glog.V(logger.Info).Infoln("====cancel",self.chain.CurrentBlock().Number())
				}
				//go self.SendViewchange(self.chain.CurrentBlock /*().NumberU64()*/ + 1)
				lid, view := self.GetConStatus()
				glog.V(logger.Info).Infoln("expire: replica=%v Send Viewchange cleader=%v curview=%d bn=%d", self.GetSelfNodeID()[:len(self.GetSelfNodeID())/16], lid[:len(lid)/16], view, self.chain.CurrentBlock /*().NumberU64()*/ +1)
			}

			//LeaseChangeByTime有等待时间，协程开启
			//go self.LeaseChangeByTime() //基于时间的租约更新
			evict.Reset(evictionInterval)
		}
	}
}

//限制区块交易数
//先选择版本，再按交易入池时间排序
func multiVSelectWithLimit2(a, b [51]int /*txs map[common.Address]TXDM.TransactionsWithTime*/) ([50]int, [50]int) {

	/*
		maxTxNum := sc.GetBlockTxsNumLimit() //区块内最大交易数
		//count := 0
		//t0 := time.Now()
		dtxs := make(TXDM.TransactionsWithTime, 0, maxTxNum)
		//objectsCounter := make(map[common.Address]int)

		for acc, accTxs := range txs {
			txLen := len(accTxs)
			if txLen == 0 { //应该不会为0

				delete(txs, acc)
				break
			}
			//objectsCounter[acc] = 0
			//FIXME 伪随机
			temp := accTxs[txLen-1] //选择最后一笔交易，一定是nonce最大的，作为分支
			dtxs = append(dtxs, temp)
			prehash := temp.PreTxHash()
			for i := txLen - 2; i >= 0; i-- { //从后往前找依赖交易
				tx := accTxs[i]
				if tx.Hash() == prehash {
					dtxs = append(dtxs, tx)
					prehash = tx.PreTxHash()
				}
			}

			if len(dtxs) >= maxTxNum {
				break
			}
		}
		//glog.V(logger.Info).Infoln("=====multiVSelect==", time.Since(t0))
		//t1 := time.Now()
		sort.Sort(TXDM.TxByTime(dtxs))
		//glog.V(logger.Info).Infoln("=====sort tx by time==",time.Since(t1))

		res := make(TXDM.Transactions, len(dtxs))
		for i, timetx := range dtxs {
			res[i] = timetx.Transaction
		}
		return res*/

	var objects_ExeSeqs [50]int
	var dependency_TX [50]int
	//dependency_TX和object_ExeSeqs分别为交易序列和交易依赖，这里暂时使用两个数组表示
	for i := 0; i < 50; i++ {
		objects_ExeSeqs[i] = a[i]
	}
	for i := 0; i < 50; i++ {
		dependency_TX[i] = b[i]
	}
	return objects_ExeSeqs, dependency_TX

}

func (worker *worker) SetMaxConsensusLifetime(t int) {
	maxConsensusLifetime = time.Second * time.Duration(t)
}

func (worker *worker) SetEvictionInterval(t int) {
	evictionInterval = time.Minute * time.Duration(t)
}
