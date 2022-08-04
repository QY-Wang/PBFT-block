package eth

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

import (
	"fmt"
	//"github.com/DWBC-ConPeer/pns"

	//"bytes"
	//"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	//"github.com/DWBC-ConPeer/accounts"
	"github.com/DWBC-ConPeer/common"
	//"github.com/DWBC-ConPeer/core"

	//"github.com/DWBC-ConPeer/core/AccContract"
	//"github.com/DWBC-ConPeer/DWVM"
	//"github.com/DWBC-ConPeer/pns"

	//"github.com/DWBC-ConPeer/core/state"
	//"github.com/DWBC-ConPeer/core/types"
	//"github.com/DWBC-ConPeer/core/types/BlockDM"
	//"github.com/DWBC-ConPeer/core/types/HeaderDM"
	//"github.com/DWBC-ConPeer/core/types/ReceiptDM"
	//"github.com/DWBC-ConPeer/core/types/TXDM"
	//"github.com/DWBC-ConPeer/core/vm"
	//"github.com/DWBC-ConPeer/ethdb"
	"github.com/DWBC-ConPeer/event"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	//"github.com/DWBC-ConPeer/node"
	//"github.com/DWBC-ConPeer/params"
	//"github.com/DWBC-ConPeer/pow"
)

type ChainConfig struct {
	String string
}

type StateDB struct {
	String string
}
type PoW struct {
	String string
}
type Set struct {
	String string
}

type Transactions struct {
	String string
}

type Transaction struct {
	String string
}

type Receipt struct {
	String string
}

type Backend struct {
	String string
}

func (self Backend) TxPool() ([51]int, [51]int) {
	var objects_ExeSeqs [51]int
	var dependency_TX [51]int
	//dependency_TX和object_ExeSeqs分别为交易序列和交易依赖，这里暂时使用两个数组表示
	for i := 0; i < 51; i++ {
		objects_ExeSeqs[i] = i + 1
	}
	for i := 0; i < 51; i++ {
		dependency_TX[i] = 50 - i
	}
	return objects_ExeSeqs, dependency_TX
}

type Database struct {
	String string
}

type Validator struct {
	String string
}

type DWCCVM struct {
	String string
}

//上面是改的
//var MinerWaitGroup = new(sync.WaitGroup)

var jsonlogger = logger.NewJsonLogger()

const (
	resultQueueSize  = 10
	miningLogAtDepth = 5
)

// Agent can register themself with the worker
type Agent interface {
	Work() chan<- *Work
	SetReturnCh(chan<- *Result)
	Stop()
	Start()
	GetHashRate() int64
}

type uint64RingBuffer struct {
	ints []uint64 //array of all integers in buffer
	next int      //where is the next insertion? assert 0 <= next < len(ints)
}

// environment is the workers current environment and holds
// all of the current state information
type Work struct {
	config           *ChainConfig
	state            *StateDB // apply state changes here
	ancestors        *Set     // ancestor set (used for checking uncle parent validity)
	family           *Set     // family set (used for checking uncle invalidity)
	uncles           *Set     // uncle set
	tcount           int      // tx count in cycle
	ownedAccounts    *Set
	lowGasTxs        Transactions
	failedTxs        Transactions
	localMinedBlocks *uint64RingBuffer // the most recent block numbers that were mined locally (used to check block inclusion)

	Block *Block // the new block

	header   *Header
	txs      []*Transaction
	receipts []*Receipt

	createdAt time.Time
}

type Result struct {
	Work  *Work
	Block *Block
}

//改：
type BlockChain struct {
	CurrentBlock uint64 //当前chain的高度
}

func (self *BlockChain) ReturnCurrentBlock() *Block {
	return &currentblock
}

type Logs struct {
}
type Node struct {
}
type ConsensuReponse struct {
	Timestamp time.Time
	Sender    string
	View      uint64
	LeaderID  string
}
type NewLeaderSet struct {
	Sender    string
	View      uint64
	Round     uint64
	Leader    string
	Votes     []*VoteForLeader
	BH        uint64
	Timestamp time.Time
}
type VoteForLeader struct {
	Sender    string
	VoteFor   string
	View      uint64
	BH        uint64
	Round     uint64
	Timestamp time.Time
	Sign      string
}

func (self *worker) SetReady(flag bool) {
	self.rmux.Lock()
	defer self.rmux.Unlock()
	self.ready = flag
}

/*
	扩充worker，以适应pbft
*/
// worker is the main object which takes care of applying messages to the new state
type worker struct {
	txNum_startconsens int //仅用于测试，构建包含一定数量的交易的区块
	config             *ChainConfig

	mu sync.Mutex

	// update loop
	mux    *event.TypeMux
	events event.Subscription
	wg     sync.WaitGroup

	agents map[Agent]struct{}
	recv   chan *Result
	pow    PoW

	eth Backend // eth chain 可以进行blockchain操作
	//改： chain *core.BlockChain
	chain *BlockChain

	proc Validator
	//chainDb ethdb.Database
	dbs map[string]Database

	coinbase common.Address // 后续用到
	gasPrice *big.Int
	extra    []byte

	currentMu sync.Mutex
	current   *Work

	uncleMu        sync.Mutex
	possibleUncles sync.Map

	txQueueMu sync.Mutex
	txQueue   sync.Map

	// atomic status counters
	mining int32
	atWork int32

	fullValidation bool
	//~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~//

	activeView bool   // view change happening
	view       uint64 // current view

	// implementation of PBFT `in`
	reqStore        sync.Map           // map[string]*core.Request    track requests
	certStore       map[msgID]*msgCert //track quorum certificates for requests
	certMux         sync.RWMutex       // cert 的读写锁
	checkpointStore sync.Map           // map[vcidx]*core.Checkpoint //track checkpoints as set
	viewChangeStore sync.Map           // map[uint64]map[string]*core.ViewChange  view sender viewchange
	newViewStore    sync.Map           // map[uint64]*core.NewView track last new-view we received or sent

	commitStore sync.Map //存储签过名的区块

	Stack   *Node      // node.Node
	stackMu sync.Mutex // stack 的互斥锁

	//~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~//

	PbftMux    *event.TypeMux
	pbftevents event.Subscription

	N int // max.number of validators in the network. replica count

	isStartSelfCheck bool //是否开启主节点自检函数
	selfcheckmux     sync.RWMutex

	quroummux sync.RWMutex // N,id and son on

	viewmux sync.RWMutex // activeView lock//FIXME 这个是否还保留？以前被删了

	conviewmux sync.RWMutex // view lock

	conPNode sync.Map //map[string]string // 许可的共识节点列表，K:nodeid，V：nodeid的sortid

	nodeID string       // 本节点的nodeid
	nidmux sync.RWMutex // nodeid lock

	//FIXME 改成并发map  去锁
	crcertstore       map[uint64]map[string]ConsensuReponse // consensus reponse certstore, format: view:sender:cr
	crcertmux         sync.RWMutex                          // cert 的读写锁
	newLeaderSetStore map[string]NewLeaderSet               ///sender/设置了新的leader
	nlStoreMux        sync.RWMutex
	//存储转发的问题

	leader    string // id of leader , use conviewmux lock
	preLeader string //use for viewchange
	votesFor  string // node votes info, use conviewmux lock

	votescert sync.Map //map[uint64]map[string]core.VoteForLeader // store the votes info

	ready bool // consensus flag
	rmux  sync.RWMutex

	electionflag bool
	electionmux  sync.RWMutex

	blockbeats     map[string]time.Time // 记录区块被共识的时间
	pvbAndBeatsMux sync.RWMutex

	PNService *PermissionedNodeService

	/*
	* 以区块为对象的共识
	 */
	// possibleValidBlock 可能正确的block，第一个key表示区块高度，第二个key表示block的hash，第二个value表示具体的block
	possibleValidBlock map[uint64]map[string]*Block

	pendingstate *BlockChain // 一个完全的副本，一个临时状态数据库，用于block的创建与验证; 记得验证或创建区块之后进行还原，验证通过后，在真的blockchain写入

	LastReqID    uint64 // request的请求标号，初始化为当前区块高度，若接收到比本地节点大的number，则更新为大的值
	hasUnDealTxs bool   // 用于判断在共识期间是否有交易入池，FIMXE 未加锁

	ccvm      *DWCCVM
	chainName string
	chkpt     bool //标志正在进行checkpoint
	chkptMux  sync.RWMutex
	initView  uint64 //记录启动时的view，用于在后面判断节点当前状态是不是刚启动状态，曾经中断过等
}

type msgID struct { // our index through certStore
	v uint64 // view
	n uint64 // reqid
}

type msgCert struct {
	prePrepare  *PrePrepare
	sentPrepare bool
	prepare     []Prepare
	sentCommit  bool
	commit      []Commit

	IsPrepre bool // whether exit  prepred
	IsPre    bool // whether exit  prepared
	IsComitt bool // whether exit  comitted
}

//var err error

//func lenOfSyncMap(val sync.Map) int {
//	len := 0
//	val.Range(func(k, v interface{}) bool {
//		len++
//		return true
//	})
//	return len
//}

// add a new param stack node.Node

/*
func newWorker(config *ChainConfig, coinbase common.Address, eth Backend, nodestack *node.Node, pbftmux *event.TypeMux, pnService *pns.PermissionedNodeService, chainName string) *worker {
	checkpointBH = eth.BlockChain().CurrentBlock().NumberU64()
	checkpointStore.Store(eth.BlockChain().CurrentBlock().Hash(), checkpointBH)
	worker := &worker{
		config: config,
		eth:    eth,
		mux:    eth.EventMux(), // 全局事件多路复用
		//chainDb:        eth.ChainDb(),
		dbs:            eth.Dbs(),
		recv:           make(chan *Result, resultQueueSize),
		gasPrice:       new(big.Int),
		chain:          eth.BlockChain(),
		proc:           eth.BlockChain().Validator(),
		coinbase:       coinbase,
		agents:         make(map[Agent]struct{}),
		fullValidation: false,

		PbftMux: pbftmux,
		//minermux: minermux, //has delete
		blockbeats:         make(map[string]time.Time),
		certStore:          make(map[msgID]*msgCert),
		checkpointStore:    sync.Map{},
		crcertstore:        make(map[uint64]map[string]ConsensuReponse),
		newViewStore:       sync.Map{},
		possibleValidBlock: make(map[uint64]map[string]*Block),
		reqStore:           sync.Map{},
		txQueue:            sync.Map{},
		viewChangeStore:    sync.Map{},
		votescert:          sync.Map{},

		pendingstate:      eth.BlockChain().Copy(), // need add Copy function in Blockchain
		PNService:         pnService,
		LastReqID:         eth.BlockChain().CurrentBlock().Number().Uint64(),
		chainName:         chainName,
		newLeaderSetStore: make(map[string]core.NewLeaderSet),
	}
	var pUncles sync.Map
	worker.possibleUncles = pUncles

	worker.view = eth.BlockChain().CurrentBlock().NumberU64() / TermNum // view 初始化
	worker.initView = worker.view
	worker.Stack = nodestack // 获取node（本地节点）的相关信息
	// TODO conPNode 改为从状态树的SystemConfig中获取
	worker.conPNode = pnService.GetPermissionedConNodes() // 获取所有许可的共识节点
	worker.nodeID = nodestack.GetLocalNodeID()            // 获取本节点的nodeid

	worker.N = lenOfSyncMap(worker.conPNode) // 共识节点数量

	worker.activeView = true // 默认为true
	worker.chkpt = false     //标志正在进行checkpoint
	worker.events = worker.mux.Subscribe(core.Checkpoint{}, core.ChainHeadEvent{}, core.ChainSideEvent{}, core.TxPreEvent{}, core.NoValidateBlock{}, core.NodeTypeChange{}, core.BeNewLeader{}, core.BlockResume{})
	worker.pbftevents = worker.PbftMux.Subscribe(core.Request{}, core.PrePrepare{}, core.Prepare{}, core.Commit{}, core.ViewChange{}, core.Checkpoint{}, core.NewView{}, core.ChainEventWorker{},
		core.PeerDisCon{}, core.PeerCon{},
		core.ConsensusRequest{}, core.ConsensuReponse{}, core.SelectionLeader{},
		core.VoteForLeader{}, core.LeaderHeartbeat{}, core.PingLeader{}, core.NewLeaderSet{},
		core.ConStatusRequest{})
	//worker.closed = make(chan bool) //FIXME 以前有，被删了，还要吗

	go worker.update()
	go worker.updatePbft()

	if worker.JudgePermissioned(worker.nodeID) {
		go worker.expirationLoop() // constatus expiration detection //FIXME
	}
	go worker.CheckConStatus() // update constatus

	return worker
}

*/
func (self *worker) setEtherbase(addr common.Address) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.coinbase = addr
}

func (self *worker) setChkpt(flag bool) {
	self.chkptMux.Lock()
	self.chkpt = flag
	self.chkptMux.Unlock()
}

func (self *worker) getChkpt() (flag bool) {
	self.chkptMux.RLock()
	defer self.chkptMux.RUnlock()
	flag = self.chkpt
	return
}

// TODO 改为从possibleValidBlocks获取数据
func (self *worker) pending() (*Block, *StateDB) {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	var a = new(Block)
	if atomic.LoadInt32(&self.mining) == 0 {
		a.Header = self.current.header
		a.Txs = self.current.txs
		a.Receipt = self.current.receipts
		return a, self.current.state
		//return BlockDM.NewBlock(
		//	self.current.header,
		//	self.current.txs,
		//	nil,
		//	self.current.receipts,
		//), self.current.state
	}
	return self.current.Block, self.current.state //.Copy()
}

// TODO checkpoint 触发日志清除等功能

/*
func (self *worker) updatePbft() {
	for event := range self.pbftevents.Chan() {
		// A real event arrived, process interesting content
		switch ev := event.Data.(type) {
		case core.Request:
			if ev.Sender == self.GetSelfNodeID() {
				glog.V(logger.Info).Infof("recv request from self reqid %d, bn %d", ev.ReqID, ev.BlockHeight)
			} else {
				glog.V(logger.Info).Infof("recv request from %v, reqid %d, bn %d", ev.Sender[:len(ev.Sender)/16], ev.ReqID, ev.BlockHeight)
				go self.RecvRequest(&ev)
			}
		case core.PrePrepare:
			if self.JudgePermissioned(ev.Sender) {
				go self.RecvPrePrepare(&ev)
			}

		case core.Prepare:
			if self.JudgePermissioned(ev.Sender) {
				go self.RecvPrepare(&ev)
			}

		case core.Commit:
			if self.JudgePermissioned(ev.Sender) {
				go self.RecvCommit(&ev)
			}

		case core.ViewChange:
			if self.JudgePermissioned(ev.Sender) {
				go self.RecvViewchange(&ev)
			}
		case core.NewLeaderSet:
			if self.JudgePermissioned(ev.Sender) {
				go self.RecvNewLeaderSet(&ev)
			}
		case core.ChainEventWorker:

		case core.PeerDisCon: // 判断是否是主节点失联了
			go self.MonitorLeader(&ev)

		case core.PeerCon:

		case core.ConsensusRequest:
			go self.RecvConsensusRequest(&ev)

		case core.ConsensuReponse:
			go self.RecvConsensuReponse(&ev)

		case core.ConStatusRequest:
			go self.RecvConStatusRequest(&ev)

		case core.SelectionLeader:
			go self.RecvSelectionLeader(&ev)

		case core.VoteForLeader:
			go self.RecvVotes(&ev)

		//case core.LeaderHeartbeat: //目前不发送该消息
		//	go self.RecvLeaderHeartbeat(&ev)

		default:
			glog.V(logger.Info).Infof("recv a unscripriton event %d", ev)

		}
	}
}*/

/*
func (self *worker) update() {
	for event := range self.events.Chan() {
		// A real event arrived, process interesting content
		switch ev := event.Data.(type) {
		case core.NodeTypeChange: // 节点类型更新
			glog.V(logger.Info).Infoln("update nodetype ... ")
			self.Stack.FindAndChangeConnByNodeID(ev.ID, ev.NodeType)

		case core.Checkpoint:
			if self.JudgePermissioned(ev.Sender) { //防止交易节点收到chainheadEvent 执行 checkpoint
				chkptGroup.Wait()
				chkptGroup.Add(1)
				self.setChkpt(true)
				go self.RecvCheckpoint(&ev)
			}
		case core.ChainHeadEvent:
			if self.PNService.ReloadPNodes(ev.Block.NumberU64()) {
				self.conPNode = self.PNService.GetPermissionedConNodes() // 获取所有许可的共识节点
				self.N = lenOfSyncMap(self.conPNode)                     // 共识节点数量
			}
			go self.mux.Post(core.Checkpoint{self.GetSelfNodeID(), ev.Block.NumberU64(), 0, ev.Block.Hash()})
		case core.NoValidateBlock:
			go self.SendViewchange(ev.BlockNumber) //FIXME 取消息中的，还是当前高度

		case core.TxPreEvent:
			// glog.V(logger.Info).Infoln("......  ....  revc TxPreEvent")
			if self.JudgePermissioned(self.GetSelfNodeID()) { //防止交易节点开启共识
				go self.StartConsensus() // 开启共识
			}
		case core.BlockResume: // 交易池删除交易并更新完成
			if self.JudgePermissioned(self.GetSelfNodeID()) && self.ISLeader() { //防止交易节点开启共识
				go self.randomwait_StartConsensus(100)
			}
		case core.BeNewLeader: // 当节点成为主节点后，需要给自己发送一条消息
			// TODO  SetConStatus 中判断leader是否是本节点，若是发送BeNewLeader消息
			//glog.V(logger.Info).Infoln("=====be new leader")
			self.ResetConData2()
			go self.randomwait_StartConsensus(250) // 开启共识

		}
	}
}*/

//改：
func (self *worker) randomwait_StartConsensus(num int) {

	if true /*self.eth.TxPool().ExistsTxs()*/ {
		t1 := time.NewTimer(time.Microsecond * time.Duration(num))
		defer t1.Stop()
		//for {
		select {
		case <-t1.C:
			if true /*self.eth.TxPool().ExistsTxs()*/ {
				go self.StartConsensus() // 开启共识
			}
			//goto anthor
		}
	}
	//anthor:
	//glog.V(logger.Info).Infoln("nothing do")
	//}
}

func newLocalMinedBlock(blockNumber uint64, prevMinedBlocks *uint64RingBuffer) (minedBlocks *uint64RingBuffer) {
	if prevMinedBlocks == nil {
		minedBlocks = &uint64RingBuffer{next: 0, ints: make([]uint64, miningLogAtDepth+1)}
	} else {
		minedBlocks = prevMinedBlocks
	}

	minedBlocks.ints[minedBlocks.next] = blockNumber
	minedBlocks.next = (minedBlocks.next + 1) % len(minedBlocks.ints)
	return minedBlocks
}

// makeCurrent creates a new environment for the current cycle.
func (self *worker) makeCurrent(parent *Block, header *Header) error {
	//state, err := self.pendingstate.State()
	////state, err := self.chain.StateAt(parent.Root())
	//if err != nil {
	//	glog.V(logger.Info).Infoln("self chain stateAt parent root %d, errr %d.", parent.Root(), err)
	//	return err
	//}
	state := StateDB{}
	set := Set{}
	work := &Work{
		config:    self.config,
		state:     &state,
		ancestors: &set,
		family:    &set,
		uncles:    &set,
		header:    header,
		createdAt: time.Now(),
		//eth:       self.eth,
	}

	// when 08 is processed ancestors contain 07 (quick block)
	//for _, ancestor := range self.chain.GetBlocksFromHash(parent.Hash(), 7) {

	/*改，ancestors等都设为空了，
	for _, ancestor := range self.pendingstate.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			work.family.Add(uncle.Hash())
		}
		work.family.Add(ancestor.Hash())
		work.ancestors.Add(ancestor.Hash())
	}*/

	//accounts := self.eth.AccountManager().Accounts()

	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	//work.ownedAccounts = accountAddressesSet(accounts)
	if self.current != nil {
		work.localMinedBlocks = self.current.localMinedBlocks
	}
	self.current = work
	return nil
}

func (self *worker) isBlockLocallyMined(current *Work, deepBlockNum uint64) bool {
	//Did this instance mine a block at {deepBlockNum} ?
	var isLocal = false
	for idx, blockNum := range current.localMinedBlocks.ints {
		if deepBlockNum == blockNum {
			isLocal = true
			current.localMinedBlocks.ints[idx] = 0 //prevent showing duplicate logs
			break
		}
	}
	//Short-circuit on false, because the previous and following tests must both be true
	if !isLocal {
		return false
	}

	//Does the block at {deepBlockNum} send earnings to my coinbase?
	var block = self.pendingstate.ReturnCurrentBlock() //.GetBlockByNumber(deepBlockNum)
	//var block = self.chain.GetBlockByNumber(deepBlockNum)
	return block != nil // && block./*.Coinbase()*/ == self.coinbase
}

func (self *worker) logLocalMinedBlocks(current, previous *Work) {
	//	glog.V(logger.Info).Infof(" for zhiqian nextBlockNum=%d", current.Block.NumberU64())

	if previous != nil && current.localMinedBlocks != nil {
		nextBlockNum := current.Block.Number //.NumberU64()
		for checkBlockNum := previous.Block.Number; /*.NumberU64()*/ checkBlockNum < nextBlockNum; checkBlockNum++ {
			inspectBlockNum := checkBlockNum - miningLogAtDepth
			if self.isBlockLocallyMined(current, inspectBlockNum) {
				glog.V(logger.Info).Infof("🔨 🔗  Mined %d blocks back: block #%v", miningLogAtDepth, inspectBlockNum)
			}
		}

	}

}

// TODO 将commitTransactions中有关交易执行与验证的地方，采用新的接口与函数，即ApplyTransaction等
/*
func (env *Work) commitTransactions(mux *event.TypeMux, txs *TXDM.TransactionsByTypeAndNonce, bc *core.BlockChain) {
	//gp := new(core.GasPool).AddGas(env.header.GetGasLimit())

	var coalescedLogs vm.Logs

	for {
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}

		// judge tx if has committed or has wrrotten into chain
		isRepeat := false
		for _, ele := range env.txs {
			if tx.Hash() == ele.Hash() {
				isRepeat = true
				break
			}
		}

		if isRepeat {
			break
		}

		// Start executing the transaction
		env.state.StartRecord(tx.Hash(), common.Hash{}, 0)

		err, logs := env.commitTransaction(tx, bc)
		switch {
		case core.IsGasLimitErr(err):
			//			// Pop the current out-of-gas transaction without shifting in the next from the account
			//			glog.V(logger.Detail).Infof("Gas limit reached for (%x) in this block. Continue to try smaller txs\n", from[:4])
			//			txs.Pop()

		case err != nil:
			// Pop the current failed transaction without shifting in the next from the account
			glog.V(logger.Detail).Infof("Transaction (%x) failed, will be removed: %v\n", tx.Hash().Bytes()[:4], err)
			env.failedTxs = append(env.failedTxs, tx)
			txs.Pop()

		default:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			env.tcount++
			txs.Shift()
		}
	}
	if len(coalescedLogs) > 0 || env.tcount > 0 {
		go func(logs vm.Logs, tcount int) {
			if len(logs) > 0 {
				mux.Post(core.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(core.PendingStateEvent{})
			}
		}(coalescedLogs, env.tcount)
	}
}*/

/*
* 基本思路：Objects_num+1个协程，每个Worker协程负责一个object的交易的执行，
* 1个主协程（Master）负责收集所有worker的执行情况
* worker将本协程已经完成的被其他worker依赖的交易（Dependency_TXDone）发送给Master，Master再将其转发给相应的worker
* worker收到Master的Dependency_TXDone消息后，判断是否所有依赖都满足，若满足则继续执行
* 当worker执行完自己的所有交易时，将ObjectDone消息发送给Master
* Master收到ObjectDone消息后，进行waitg.Done()等工作
 */

const (
	Dependency_TXDone   = "Dependency_TXDone"
	TXDone              = "TXDone"
	ObjectDone          = "ObjectDone"
	TXFailed            = "TXFailed"
	Dependency_TXFailed = "Dependency_TXFailed"
)

type Msg struct {
	Type     string
	OID      common.Address
	TxID     int
	Logs     Logs
	Receipts map[int]*Receipt
	Receipt  *Receipt
	//Txs map[int]*TXDM.Transaction
}

//var Dependency_TX //FIXME 所有的依赖关系原来是string/int// 交易依赖，txid/txid/oid交易被那些交易依赖,第二层的value表示oid
// 每一个Object的执行序列，int表示对象ID；
/*
func (env *Work) commitTransactions_parallel_1(mux *event.TypeMux, txs TXDM.Transactions, bc *core.BlockChain,
	Objects_ExeSeqs map[common.Address]core.Object_ExeSeq, Dependency_TX map[int]map[common.Address]int) {

	//stopper := profile.Start(profile.MemProfile, profile.ProfilePath("./committx"))
	//defer stopper.Stop()

	//如果是空区块直接返回
	if len(txs) == 0 {
		return
	}
	//FIXME 主协程共享变量，是否需要传递
	objects_num := len(Objects_ExeSeqs)          // 要执行的object数量
	var coalescedLogs vm.Logs                    //logs收集
	receipts := make(map[int]*ReceiptDM.Receipt) //receipts收集

	// 数据初始化
	var WorkerChannels sync.Map     //make(map[common.Address]chan Msg) // Object_ExeSeq的channel，int表示objectid
	MasterChannel := make(chan Msg) // MasterChannel
	//记录被关闭的workerChannel，避免masterchan 给已关闭channel发消息
	WorkerChanneClosed := make(map[common.Address]bool, len(Objects_ExeSeqs))
	//初始化 workerChannel
	for oid, _ := range Objects_ExeSeqs {
		WorkerChannels.Store(oid, make(chan Msg))
	}

	// 进程划分
	var waitg sync.WaitGroup
	waitg.Add(objects_num + 1) // one master thread, objects_num worker thread

	glog.V(logger.Info).Infoln("--start-mastercthread, txlen:", len(txs))
	// master cothread
	go func(WorkerChannels sync.Map, MasterChannel chan Msg, txs TXDM.Transactions,
		Dependency_TX map[int]map[common.Address]int, waitg *sync.WaitGroup) {

		counter := 0 // 已完成的object的计数器

		for {
			select {
			case msg := <-MasterChannel:

				if msg.Type == Dependency_TXDone || msg.Type == Dependency_TXFailed {
					//if msg.Type == Dependency_TXFailed{
					//	glog.V(logger.Info).Infoln("=== DFailed",msg.TxID)
					//}
					if dobjs, ok := Dependency_TX[msg.TxID]; ok {
						//if msg.Type == Dependency_TXFailed{
						//	glog.V(logger.Info).Infoln("=== DFailed",msg.TxID)
						//}
						for oid, _ := range dobjs {
							//未关闭则发送，此处加判断，因为存在failed后，整个分支都failed，即使依赖交易没到，也会objdone，closechan
							if !WorkerChanneClosed[oid] {
								wch, _ := WorkerChannels.Load(oid) // 获取指定OID的channel
								wch.(chan Msg) <- msg
							}

						}
					}
				}

				if msg.Type == ObjectDone {

					// 某个object的所有交易已处理完成
					//glog.V(logger.Info).Infoln("--------master receive ObjectDone----------")

					for txid, receipt := range msg.Receipts {
						receipts[txid] = receipt
					}
					coalescedLogs = append(coalescedLogs, msg.Logs...)

					waitg.Done()
					//glog.V(logger.Info).Infoln("--------worker wait done----------", msg.OID.Hex())

					WorkerChanneClosed[msg.OID] = true
					wch, _ := WorkerChannels.Load(msg.OID) // 获取指定OID的channel
					go close(wch.(chan Msg))

					counter++
					if counter == objects_num {
						waitg.Done() //最后一个
						//glog.V(logger.Info).Infoln("--------master wait done----------")

						goto Loop
					}
				}
			}
		}
	Loop:
	}(WorkerChannels, MasterChannel, txs, Dependency_TX, &waitg)

	//glog.V(logger.Info).Infoln("--ctp-worker cothread")
	// worker cothread //FIXME txs是共享变量
	for oid, oe := range Objects_ExeSeqs {
		wch, _ := WorkerChannels.Load(oid)
		go func(wch chan Msg, MasterChannel chan Msg, oid common.Address, oe core.Object_ExeSeq, txs TXDM.Transactions) {

			//记录该worker执行交易的receipt key交易序号txid，value返回的receipt
			Receipts := make(map[int]*ReceiptDM.Receipt)
			var Log vm.Logs

			objTxLen := len(oe)

			execuing := -1 //正在执行的txid，初始化为-1
			// 判断第一笔交易是否可执行
			if len(oe[0].Dependency) == 0 {
				txid := oe[0].TX
				execuing++
				go env.executor_1(txid, txs[txid], bc, wch) // 执行交易execNum
			}

			//success := 0 //已成功的交易计数器
			counter := 0 // 已处理的交易的计数器
			for {
				select {
				case msg := <-wch:
					if msg.Type == TXDone || msg.Type == TXFailed {
						//执行成功
						// 0、记录传回来的值
						// 1、counter++
						// 2、通知主进程有依赖
						// 3、根据counter判断本组是否执行完毕，完毕发送objectDone消息
						// 5、没有完毕，则判断自己的下一个能否执行，能执行则开启

						//glog.V(logger.Info).Infoln("--------receive TXDone----------", msg.TxID)

						//记录传回来的值
						Receipts[msg.TxID] = msg.Receipt
						Log = append(Log, msg.Logs...)

						counter++
						if oe[counter-1].BeDependent { //如果被依赖
							//glog.V(logger.Info).Infoln("--------BeDependent----------", msg.TxID)

							MasterChannel <- Msg{
								Type: "Dependency_" + msg.Type,
								TxID: msg.TxID,
							}
						}

						//glog.V(logger.Info).Infoln("--------",counter, txLen)

						if counter == objTxLen {
							//glog.V(logger.Info).Infoln("--------worker objDone----------", msg.TxID)

							MasterChannel <- Msg{Type: ObjectDone, OID: oid, Receipts: Receipts, Logs: Log}
							goto Loop
						}

						//开启下一个执行
						tx := oe[counter]
						if len(tx.Dependency) == 0 {
							//glog.V(logger.Info).Infoln("--------do next----------", tx.TX)

							txid := tx.TX
							execuing++
							go env.executor_1(txid, txs[txid], bc, wch) // 执行交易executing_Num + 1
						}
					}

					if msg.Type == Dependency_TXDone || msg.Type == Dependency_TXFailed {
						//依赖交易执行完
						// 1、遍历该obj组，移除对应的依赖交易
						// 2、查看下一个交易是否还有依赖交易，没有则开启下一个执行

						//glog.V(logger.Info).Infoln("--------worker Dependency_TXDone----------", msg.TxID)

						for _, tx := range oe {
							//if _, ok := Dependency_TX[msg.TxID][tx.TX]; ok {
							delete(tx.Dependency, msg.TxID)
							//break// FIXME 应该从success 开始 应该删除一个就好一个 目前不能保证后面没设置该依赖
							//}
						}

						if counter != execuing { //表示下一笔交易未开始执行
							if len(oe[counter].Dependency) == 0 {
								txid := oe[counter].TX
								execuing++
								go env.executor_1(txid, txs[txid], bc, wch) // 执行交易success
							}
						}
					}
				}
			}
		Loop:
		}(wch.(chan Msg), MasterChannel, oid, oe, txs)
	}
	waitg.Wait() // 等待所有的完成
	go close(MasterChannel)
	glog.V(logger.Info).Infoln("--end-worker cothread")
	//执行env
	for txid, tx := range txs {
		if receipt, ok := receipts[txid]; ok {
			env.txs = append(env.txs, tx)
			env.receipts = append(env.receipts, receipt)
		} else {
			glog.V(logger.Error).Infoln("execute err", tx.Hash().Hex())
		}
	}

	env.tcount = env.tcount + len(receipts)

	//glog.V(logger.Info).Infoln("--ctp waitg.Wait() after----------")
	//time.Sleep(time.Second)
	if len(coalescedLogs) > 0 || env.tcount > 0 {
		go func(logs vm.Logs, tcount int) {
			if len(logs) > 0 {
				mux.Post(core.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(core.PendingStateEvent{})
			}
		}(coalescedLogs, env.tcount)
	}
}*/

//改：注释了
//func (env *Work) executor_1(txid int, tx *TXDM.Transaction, bc *core.BlockChain, wch chan Msg) {
//	//glog.V(logger.Info).Infoln("--------txid", txid, tx.Object().Hex())
//	//FIXME 目前的模式，下面的错误应该不会发生
//	//交易不应该为nil，如果为nil，自己panic
//	//交易不应该重复，重复则系统逻辑有问题直接panic
//	for _, ele := range env.txs {
//		if tx.Hash() == ele.Hash() {
//			panic("tx is repeat")
//		}
//	}
//	// Start executing the transaction
//	env.state.StartRecord(tx.Hash(), common.Hash{}, 0)
//	//==========================================================
//	//	err, logs := env.commitTransaction(tx, bc)
//
//	// FIXME 虚拟机用，暂时注释
//	// this is a bit of a hack to force jit for the miners
//	config := env.config.VmConfig
//	//if !(config.EnableJit && config.ForceJit) {
//	//	config.EnableJit = false
//	//}
//	//config.ForceJit = false // disable forcing jit
//
//	receipt, logs, err := core.ApplyTransaction(env.config, bc, env.state, env.header, tx /*env.header.GasUsed*/, config)
//
//	if err != nil {
//		//删除statedb.ms中所有tx.hash 版本的对象，并将该对象的最新状态置为pre版本
//		//env.state.RevertDbVersion(tx.Hash())
//		// FIXME 如果失败了，是不是不需要回滚了
//		// 1、如果回滚，配额扣减会不成功
//		// 2、如果出错是在配额扣减之前，会panic，不会走到这里
//		// 3、如果err 是在配额扣减之后，如果是检验错误，不需要回滚
//		// 4、如果是执行错误，不会更新statedb，不需要回滚【因为执行单元中，没有对from的ptoken执行】
//	}
//
//	//master 执行
//	//env.txs = append(env.txs, tx)
//	//env.receipts = append(env.receipts, receipt)
//
//	//===============================================
//
//	switch {
//	case core.IsGasLimitErr(err):
//		//			// Pop the current out-of-gas transaction without shifting in the next from the account
//		//			glog.V(logger.Detail).Infof("Gas limit reached for (%x) in this block. Continue to try smaller txs\n", from[:4])
//		//			txs.Pop()
//		glog.V(logger.Info).Infoln("======== 0 ===", txid, err) //共识主节点调试
//
//		wch <- Msg{TxID: txid, Type: TXFailed, Receipt: receipt, Logs: logs}
//
//		return
//	case err != nil:
//		// Pop the current failed transaction without shifting in the next from the account
//		glog.V(logger.Detail).Infof("Transaction (%x) failed, will be removed: %v\n", tx.Hash().Bytes()[:4], err)
//
//		//env.failedTxs = append(env.failedTxs, tx) //master执行
//		glog.V(logger.Info).Infoln("======== 0 ===", txid, err) //共识主节点调试
//
//		wch <- Msg{TxID: txid, Type: TXFailed, Receipt: receipt, Logs: logs}
//
//		//glog.V(logger.Info).Infoln("-----txfailed----------")
//		return
//
//	default:
//		// Everything ok, collect the logs and shift in the next transaction from the same account
//
//		//master执行
//		//coalescedLogs = append(coalescedLogs, logs...)
//		//env.tcount++
//		//glog.V(logger.Info).Infoln("--------executor TXDone----------", txid)
//		glog.V(logger.Info).Infoln("======== 1 ===", txid) //共识主节点调试
//
//		wch <- Msg{TxID: txid, Type: TXDone, Receipt: receipt, Logs: logs}
//
//		return
//	}
//}

type TXDoneMsg struct {
	//Type     string
	//OID      common.Address
	Err  error
	TxID int
	Logs Logs
	//Receipts map[int]*ReceiptDM.Receipt
	Receipt *Receipt
	//Txs map[int]*TXDM.Transaction
} //worker自己发给worker

type ObjectDoneMsg struct {
	//Type     string
	OID string
	//TxID     int
	Logs     Logs
	Receipts map[int]*Receipt
	//Receipt  *ReceiptDM.Receipt
	//Txs map[int]*TXDM.Transaction
} //worker发给master

type DependencyDoneMsg struct {
	//Type     string
	//OID      common.Address
	TxID int
	//Logs     vm.Logs
	//Receipts map[int]*ReceiptDM.Receipt
	//Receipt  *ReceiptDM.Receipt
	//Txs map[int]*TXDM.Transaction
} //worker发给master，master发给worker

//var Dependency_TX //FIXME 所有的依赖关系原来是string/int// 交易依赖，txid/txid/oid交易被那些交易依赖,第二层的value表示oid
// 每一个Object的执行序列，int表示对象ID；

//改：验证取得的交易是否合法
func (env *Work) commitTransactions_parallel(mux *event.TypeMux, /* txs TXDM.Transactions, bc *core.BlockChain,
	Objects_ExeSeqs map[string]core.Object_ExeSeq, Dependency_TX map[int]map[string]int*/txs string, bc *BlockChain,
	Objects_ExeSeqs [50]int, Dependency_TX [50]int) bool {

	//改：
	a := mux
	b := txs
	c := bc
	d := Objects_ExeSeqs
	e := Dependency_TX
	fmt.Println(a, b, c, d, e)
	//人工修改交易是否合法
	return true

	//stopper := profile.Start(profile.MemProfile, profile.ProfilePath("./committx"))
	//defer stopper.Stop()

	//如果是空区块直接返回
	//if len(txs) == 0 {
	//	return
	//}
	////FIXME 主协程共享变量，是否需要传递
	//objects_num := len(Objects_ExeSeqs)          // 要执行的object数量
	//var coalescedLogs vm.Logs                    //logs收集
	//receipts := make(map[int]*ReceiptDM.Receipt) //receipts收集
	//
	//// 数据初始化
	//MasterMux := new(event.TypeMux) // MasterChannel
	//MasterEvent := MasterMux.Subscribe(ObjectDoneMsg{}, DependencyDoneMsg{})
	//
	//var WorkerMuxs sync.Map //make(map[common.Address]chan Msg) // Object_ExeSeq的channel，int表示objectid
	//var WorkerEvents sync.Map
	//
	////初始化 workerChannel
	//for oid, _ := range Objects_ExeSeqs {
	//	workerMux := new(event.TypeMux)
	//	WorkerMuxs.Store(oid, workerMux)
	//	workerEvent := workerMux.Subscribe(TXDoneMsg{}, DependencyDoneMsg{})
	//	WorkerEvents.Store(oid, workerEvent)
	//}
	//
	//// 进程划分
	//var waitg sync.WaitGroup
	//waitg.Add(objects_num + 1) // one master thread, objects_num worker thread
	//
	//glog.V(logger.Info).Infoln("--start-mastercthread, txlen:", len(txs))
	//// master cothread
	//go func(mevent event.Subscription, workerMux, wokerEvents sync.Map, txs TXDM.Transactions,
	//	Dependency_TX map[int]map[string]int, waitg *sync.WaitGroup) {
	//
	//	counter := 0 // 已完成的object的计数器
	//
	//	for ev := range mevent.Chan() {
	//		// A real event arrived, process interesting content
	//		//glog.V(logger.Info).Infoln("---m recv msg---", ev)
	//		switch msg := ev.Data.(type) {
	//		case DependencyDoneMsg:
	//			if dobjs, ok := Dependency_TX[msg.TxID]; ok {
	//				for oid, _ := range dobjs {
	//					//glog.V(logger.Info).Infoln("====m recv depend", msg.TxID, "bedepend----", oid.Hex())
	//					temp, _ := workerMux.Load(oid)
	//					wmux := temp.(*event.TypeMux)
	//					go wmux.Post(msg)
	//					//glog.V(logger.Info).Infoln("====m after send depend", msg.TxID, "----", oid.Hex())
	//				}
	//			} else {
	//				//glog.V(logger.Info).Infoln("====m recv depend", msg.TxID, "not bedepend")
	//			}
	//
	//		case ObjectDoneMsg:
	//
	//			// 某个object的所有交易已处理完成
	//			//glog.V(logger.Info).Infoln("====m recv objdone====", msg.OID.Hex())
	//
	//			for txid, receipt := range msg.Receipts {
	//				receipts[txid] = receipt
	//			}
	//			coalescedLogs = append(coalescedLogs, msg.Logs...)
	//
	//			waitg.Done()
	//
	//			counter++
	//			if counter == objects_num {
	//				waitg.Done() //最后一个
	//				goto Loop
	//			}
	//
	//		default:
	//			glog.V(logger.Info).Infof("recv a unscripriton event %d", msg)
	//		}
	//	}
	//Loop:
	//}(MasterEvent, WorkerMuxs, WorkerEvents, txs, Dependency_TX, &waitg)
	//
	//// worker cothread //FIXME txs是共享变量
	//for oid, oe := range Objects_ExeSeqs {
	//	wch, _ := WorkerMuxs.Load(oid)
	//	wevent, _ := WorkerEvents.Load(oid)
	//	go func(wmux, mmux *event.TypeMux, wevent event.Subscription, oid string, oe core.Object_ExeSeq, txs TXDM.Transactions) {
	//
	//		//记录该worker执行交易的receipt key交易序号txid，value返回的receipt
	//		Receipts := make(map[int]*ReceiptDM.Receipt)
	//		var Log vm.Logs
	//
	//		objTxLen := len(oe)
	//
	//		execuing := -1 //正在执行的txid，初始化为-1
	//		// 判断第一笔交易是否可执行
	//		if len(oe[0].Dependency) == 0 {
	//			txid := oe[0].TX
	//			execuing++
	//			go env.executor(txid, txs[txid], bc, wmux) // 执行交易execNum
	//		}
	//
	//		counter := 0 // 已处理的交易的计数器
	//		for ev := range wevent.Chan() {
	//			// A real event arrived, process interesting content
	//
	//			switch msg := ev.Data.(type) {
	//			case TXDoneMsg:
	//				//执行成功
	//				// 0、记录传回来的值
	//				// 1、counter++
	//				// 2、通知主进程有依赖
	//				// 3、根据counter判断本组是否执行完毕，完毕发送objectDone消息
	//				// 5、没有完毕，则判断自己的下一个能否执行，能执行则开启
	//
	//				if msg.Err != DWVM.ErrMultiVersion { //如果不是多版本错误，则封入区块中
	//					//记录传回来的值
	//					Receipts[msg.TxID] = msg.Receipt
	//					Log = append(Log, msg.Logs...)
	//				}
	//
	//				counter++
	//				if oe[counter-1].BeDependent { //如果被依赖
	//					//glog.V(logger.Info).Infoln("--------BeDependent----------", msg.TxID)
	//					//if counter == objTxLen{
	//					//	glog.V(logger.Info).Infoln("----------","end", "--------w recv txdone-----bedep-----", msg.TxID, oe[counter-1].BeDependent)
	//					//}else{
	//					//	glog.V(logger.Info).Infoln("----------",oe[counter].TX, "--------w recv txdone-----bedep-----", msg.TxID, oe[counter-1].BeDependent)
	//					//}
	//
	//					go mmux.Post(DependencyDoneMsg{msg.TxID})
	//					//if counter == objTxLen{
	//					//	glog.V(logger.Info).Infoln("----------","end", "--------w after send depend------", msg.TxID, oe[counter-1].BeDependent)
	//					//}else{
	//					//	glog.V(logger.Info).Infoln("----------",oe[counter].TX, "--------w after send depend------", msg.TxID, oe[counter-1].BeDependent)
	//					//}
	//				} else {
	//					//if counter == objTxLen{
	//					//	glog.V(logger.Info).Infoln("----------","end", "--------w recv txdone----not bedep-----", msg.TxID, oe[counter-1].BeDependent)
	//					//}else{
	//					//	glog.V(logger.Info).Infoln("----------",oe[counter].TX, "--------w recv txdone-----not bedep-----", msg.TxID, oe[counter-1].BeDependent)
	//					//}
	//				}
	//
	//				if counter == objTxLen {
	//					go mmux.Post(ObjectDoneMsg{OID: oid, Receipts: Receipts, Logs: Log})
	//					goto Loop
	//				}
	//
	//				//开启下一个执行
	//				tx := oe[counter]
	//				if len(tx.Dependency) == 0 {
	//					//glog.V(logger.Info).Infoln("--------do next----------", tx.TX)
	//
	//					txid := tx.TX
	//					execuing++
	//					go env.executor(txid, txs[txid], bc, wmux) // 执行交易executing_Num + 1
	//				}
	//
	//			case DependencyDoneMsg:
	//				//依赖交易执行完
	//				// 1、遍历该obj组，移除对应的依赖交易
	//				// 2、查看下一个交易是否还有依赖交易，没有则开启下一个执行
	//
	//				//if counter == objTxLen{
	//				//	glog.V(logger.Info).Infoln("-------------","end", "--------w recv depend----------", msg.TxID)
	//				//}else{
	//				//	glog.V(logger.Info).Infoln("-------------",oe[counter].TX, "--------w recv depend----------", msg.TxID)
	//				//}
	//				for _, tx := range oe {
	//					//if _, ok := Dependency_TX[msg.TxID][tx.TX]; ok {
	//					delete(tx.Dependency, msg.TxID)
	//					//break// FIXME 应该从success 开始 应该删除一个就好一个 目前不能保证后面没设置该依赖
	//					//}
	//				}
	//
	//				if counter != execuing { //表示下一笔交易未开始执行
	//					if len(oe[counter].Dependency) == 0 {
	//						txid := oe[counter].TX
	//						execuing++
	//						go env.executor(txid, txs[txid], bc, wmux) // 执行交易success
	//					}
	//				}
	//
	//			default:
	//				glog.V(logger.Info).Infof("recv a unscripriton event %d", msg)
	//			}
	//		}
	//	Loop:
	//	}(wch.(*event.TypeMux), MasterMux, wevent.(event.Subscription), oid, oe, txs)
	//}
	//waitg.Wait() // 等待所有的完成
	//
	//WorkerEvents.Range(func(k, v interface{}) bool {
	//	wev := v.(event.Subscription)
	//	wev.Unsubscribe()
	//	return true
	//})
	//
	//WorkerMuxs.Range(func(k, v interface{}) bool {
	//	wmux := v.(*event.TypeMux)
	//	wmux.Stop()
	//	return true
	//})
	//
	//MasterEvent.Unsubscribe()
	//MasterMux.Stop()
	//
	//glog.V(logger.Info).Infoln("--end-worker cothread")
	////执行env
	//count := 0
	//for txid, tx := range txs {
	//	if receipt, ok := receipts[txid]; ok {
	//		env.txs = append(env.txs, tx)
	//		env.receipts = append(env.receipts, receipt)
	//	} else {
	//		count++
	//		//glog.V(logger.Error).Infoln("execute err", tx.Hash().Hex())
	//	}
	//}
	//if count != 0 {
	//	glog.V(logger.Error).Infoln("execute err txcount", count)
	//}
	//
	//env.tcount = env.tcount + len(receipts)
	//
	//if len(coalescedLogs) > 0 || env.tcount > 0 {
	//	go func(logs vm.Logs, tcount int) {
	//		if len(logs) > 0 {
	//			go mux.Post(core.PendingLogsEvent{Logs: logs})
	//		}
	//		if tcount > 0 {
	//			go mux.Post(core.PendingStateEvent{})
	//		}
	//	}(coalescedLogs, env.tcount)
	//}

}

//
//func (env *Work) executor(txid int, tx *TXDM.Transaction, bc *core.BlockChain, wch *event.TypeMux) {
//
//	//glog.V(logger.Info).Infoln("-------------------start", txid)
//	//FIXME 目前的模式，下面的错误应该不会发生
//	//交易不应该为nil，如果为nil，自己panic
//	//交易不应该重复，重复则系统逻辑有问题直接panic
//	for _, ele := range env.txs {
//		if tx.Hash() == ele.Hash() {
//			panic("tx is repeat")
//		}
//	}
//	// Start executing the transaction
//	env.state.StartRecord(tx.Hash(), common.Hash{}, 0)
//	//==========================================================
//	//	err, logs := env.commitTransaction(tx, bc)
//
//	// FIXME 虚拟机用，暂时注释
//	// this is a bit of a hack to force jit for the miners
//	config := env.config.VmConfig
//	//if !(config.EnableJit && config.ForceJit) {
//	//	config.EnableJit = false
//	//}
//	//config.ForceJit = false // disable forcing jit
//
//	receipt, logs, err := core.ApplyTransaction(env.config, bc, env.state, env.header, tx /*env.header.GasUsed*/, config)
//
//	if err != nil {
//		//删除statedb.ms中所有tx.hash 版本的对象，并将该对象的最新状态置为pre版本
//		//env.state.RevertDbVersion(tx.Hash())
//		// FIXME 如果失败了，是不是不需要回滚了
//		// 1、如果回滚，配额扣减会不成功
//		// 2、如果出错是在配额扣减之前，会panic，不会走到这里
//		// 3、如果err 是在配额扣减之后，如果是检验错误，不需要回滚
//		// 4、如果是执行错误，不会更新statedb，不需要回滚【因为执行单元中，没有对from的ptoken执行】
//	}
//
//	//master 执行
//	//env.txs = append(env.txs, tx)
//	//env.receipts = append(env.receipts, receipt)
//
//	//===============================================
//
//	switch {
//	case core.IsGasLimitErr(err):
//		//			// Pop the current out-of-gas transaction without shifting in the next from the account
//		//			glog.V(logger.Detail).Infof("Gas limit reached for (%x) in this block. Continue to try smaller txs\n", from[:4])
//		//			txs.Pop()
//		glog.V(logger.Info).Infoln("======== 0 ===", txid, err) //共识主节点调试
//
//		go wch.Post(TXDoneMsg{TxID: txid, Receipt: receipt, Logs: logs, Err: err})
//
//		return
//	case err != nil:
//		// Pop the current failed transaction without shifting in the next from the account
//		glog.V(logger.Detail).Infof("Transaction (%x) failed, will be removed: %v\n", tx.Hash().Bytes()[:4], err)
//
//		//env.failedTxs = append(env.failedTxs, tx) //master执行
//		if err != DWVM.ErrMultiVersion {
//			glog.V(logger.Info).Infoln("======== 0 ===", txid, err) //共识主节点调试
//		}
//		go wch.Post(TXDoneMsg{TxID: txid, Receipt: receipt, Logs: logs, Err: err})
//		return
//
//	default:
//		// Everything ok, collect the logs and shift in the next transaction from the same account
//
//		//master执行
//		//coalescedLogs = append(coalescedLogs, logs...)
//		//env.tcount++
//		//glog.V(logger.Info).Infoln("======== 1 ===", txid)//共识主节点调试
//
//		go wch.Post(TXDoneMsg{TxID: txid, Receipt: receipt, Logs: logs, Err: err})
//
//		return
//	}
//}

//func (env *Work) commitTransaction(tx *TXDM.Transaction, bc *core.BlockChain) (error, vm.Logs) {
//	//snap := env.state.Snapshot()
//
//	// this is a bit of a hack to force jit for the miners
//	config := env.config.VmConfig
//	if !(config.EnableJit && config.ForceJit) {
//		config.EnableJit = false
//	}
//	config.ForceJit = false // disable forcing jit
//
//	receipt, logs, err := core.ApplyTransaction(env.config, bc, env.state, env.header, tx /*env.header.GasUsed*/, config)
//	//if err != nil {
//	//	env.state.RevertToSnapshot(snap)
//	//	return err, nil
//	//}
//
//	if err != nil {
//		//删除statedb.ms中所有tx.hash 版本的对象，并将该对象的最新状态置为pre版本
//		env.state.RevertDbVersion(tx.Hash())
//		return err, nil
//	}
//	env.txs = append(env.txs, tx)
//	env.receipts = append(env.receipts, receipt)
//
//	return nil, logs
//}
//
//func accountAddressesSet(accounts []accounts.Account) *set.Set {
//	accountSet := set.New()
//	for _, account := range accounts {
//		accountSet.Add(account.Address)
//	}
//	return accountSet
//}

func (instance *worker) removeOldConMsg(reqID uint64) {
	instance.certMux.Lock()
	for msgId, _ := range instance.certStore {
		if msgId.n <= reqID {
			delete(instance.certStore, msgId)
		}
	}
	instance.certMux.Unlock()

	instance.reqStore.Range(func(k, v interface{}) bool {
		if v.(*Request).ReqID <= reqID {
			instance.reqStore.Delete(k)
		}
		return true
	})
}

//清空共识状态，恢复参数到程序初始化状态
func (self *worker) ResetConData() {
	self.conviewmux.Lock()
	self.view = self.chain.CurrentBlock //().NumberU64() / TermNum // view 初始化
	self.conviewmux.Unlock()
	self.ResetCommitStore()
	self.SetReady(false) // 未选举
	self.SetActiveView(true)

	self.reqStore.Range(func(k, v interface{}) bool {
		self.reqStore.Delete(k)
		return true
	})
	self.ResetPVBAndBeats()
	self.certMux.Lock()
	for k, _ := range self.certStore {
		delete(self.certStore, k)
	}
	self.certMux.Unlock()
	self.checkpointStore.Range(func(k, v interface{}) bool {
		self.checkpointStore.Delete(k)
		return true
	})
	if consensusCancel != nil {
		consensusCancel()
	}

	//go self.CheckConStatus() // update constatus
}

//miner内部调用
func (self *worker) ResetConData2() {
	//self.view = self.chain.CurrentBlock().NumberU64() / TermNum // view 初始化
	//self.SetReady(false)     // 未选举
	//self.SetActiveView(true)

	self.reqStore.Range(func(k, v interface{}) bool {
		self.reqStore.Delete(k)
		return true
	})
	self.ResetPVBAndBeats()
	self.certMux.Lock()
	for k, _ := range self.certStore {
		delete(self.certStore, k)
	}
	self.certMux.Unlock()
	self.checkpointStore.Range(func(k, v interface{}) bool {
		self.checkpointStore.Delete(k)
		return true
	})

	if consensusCancel != nil {
		consensusCancel()
	}

	//go self.CheckConStatus() // update constatus
}

func (self *worker) ForTestMem() {
}

func (self *worker) SetTxNum_startconsens(num int) {
	self.txNum_startconsens = num
}
