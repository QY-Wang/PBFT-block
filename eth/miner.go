package eth

import (
	"errors"
	"fmt"
	"github.com/DWBC-ConPeer/common"
	"github.com/DWBC-ConPeer/event"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/params"
	"math/big"
	"sync/atomic"
)

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

// Package miner implements Ethereum block creation and mining.

type PermissionedNodeService struct {
}
type Miner struct {
	mux *event.TypeMux

	worker *worker

	MinAcceptedGasPrice *big.Int

	threads  int
	coinbase common.Address
	mining   int32
	eth      Backend
	pow      PoW

	canStart    int32          // can start indicates whether we can start the mining operation
	shouldStart int32          // should start indicates whether we should start after sync
	Stack       *Node          // node.Node 属性，获取链接peer
	PbftMux     *event.TypeMux //channel of worker and protocolManger
	minermux    *event.TypeMux //channerl of worker and miner
	minerevent  event.Subscription
	PNService   *PermissionedNodeService
}

func (self *Miner) GetPbftMux() *event.TypeMux {
	return self.PbftMux
}
func (self *Miner) GetMinerMux() *event.TypeMux {
	return self.minermux
}

// add a new param stack node.Node
// add a new param PNService pns.PermissionedNodeService by wmx at 20190312

//改：先注释
/*
func New(eth core.Backend, config *core.ChainConfig, mux *event.TypeMux, pow pow.PoW, stack *node.Node, pbftmux *event.TypeMux, pnService *pns.PermissionedNodeService, chainName string) *Miner {
	minermux := new(event.TypeMux) // 新建一个channel，用于 minerstart 与 minerstop 事件通信
	miner := &Miner{
		eth:       eth,
		mux:       mux,
		pow:       pow,
		worker:    newWorker(config, stack.GetNodeAccount(), eth, stack, pbftmux, pnService, chainName),
		canStart:  1,
		Stack:     stack,
		PbftMux:   pbftmux,
		minermux:  minermux,
		PNService: pnService,
	}
	miner.minerevent = miner.minermux.Subscribe(core.NoValidateBlock{}, core.MinerStartEvent{}, core.MinerStopEvent{}, core.ChainEventMiner{}, core.UpdateWork{})
	miner.coinbase = miner.Stack.GetNodeAccount() // initial coinbase, coinbase
	go miner.update()
	go miner.updateStart()
	return miner
}
*/

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
/*func (self *Miner) update() {

	events := self.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
out:
	for ev := range events.Chan() {
		switch ev.Data.(type) {
		case downloader.StartEvent: //注释掉下面的原因是，只有leader在挖矿，而leader的区块应该是最高的，应该不会需要downloader
			//atomic.StoreInt32(&self.canStart, 0)
			//if self.Mining() {
			//	self.Stop()
			//	atomic.StoreInt32(&self.shouldStart, 1)
			//	glog.V(logger.Info).Infoln("Mining operation aborted due to sync operation")
			//}
			glog.V(logger.Info).Infoln("downloader StartEvent")

		case downloader.DoneEvent, downloader.FailedEvent:
			//shouldStart := atomic.LoadInt32(&self.shouldStart) == 1
			////			glog.V(logger.Info).Infoln("... downloader.DoneEvent, downloader.FailedEvent ...")
			//atomic.StoreInt32(&self.canStart, 1)
			//atomic.StoreInt32(&self.shouldStart, 0)
			//if shouldStart {
			//	self.Start(self.coinbase, self.threads, self.Stack)
			//}
			// unsubscribe. we're only interested in this event once
			events.Unsubscribe()
			// stop immediately and ignore all further pending events
			glog.V(logger.Info).Infoln("downloader DoneEvent or downloader FailedEvent")

			break out

		}
	}
}*/

//没用了
/*
func (self *Miner) updateStart() {

	for event := range self.minerevent.Chan() {
		//switch ev := event.Data.(type) {
		switch event.Data.(type) {
		// MinerstartEvent 处理
		case core.MinerStartEvent:
			//			if !self.Mining() {
			//				self.Start(self.coinbase, 1, self.Stack)
			//			}

		case core.MinerStopEvent:
			//			glog.V(logger.Info).Infoln("... miner stop ...")
			//			self.Stop()
		case core.UpdateWork:
			//			self.Start(self.coinbase, 0, self.Stack)
			//			self.Stop()
		case core.ChainEventMiner:
			//			self.Stop()
			//			cew := core.ChainEventWorker{
			//				Block: ev.Block,
			//			}
			//			self.PbftMux.Post(cew)
		case core.NoValidateBlock:
			//			self.minermux.Post(ev)
		}
	}
}*/

//func (m *Miner) SetGasPrice(price *big.Int) {
//	// FIXME block tests set a nil gas price. Quick dirty fix
//	if price == nil {
//		return
//	}
//
//	m.worker.setGasPrice(price)
//}

//func (self *Miner) Start(coinbase common.Address, threads int, stack *node.Node) {
//	glog.V(logger.Info).Infoln("... miner start ...")
//	atomic.StoreInt32(&self.shouldStart, 1)
//	self.threads = threads
//	self.worker.coinbase = coinbase
//	self.coinbase = coinbase
//	self.worker.Stack = stack
//	//	glog.V(logger.Info).Infoln(" stack is stack")
//	//	self.worker.Resetworker()
//	if atomic.LoadInt32(&self.canStart) == 0 {
//		glog.V(logger.Info).Infoln("Can not start mining operation due to network sync (starts when finished)")
//		return
//	}

//FIXME 为不影响backend等调用，暂时保留空方法
func (self *Miner) Start(coinbase common.Address, threads int, stack *Node) {
}

//func (self *Miner) Start(coinbase common.Address, threads int, stack *node.Node) {
//	glog.V(logger.Info).Infoln("... miner start ...")
//	atomic.StoreInt32(&self.shouldStart, 1)
//	self.threads = threads
//	self.worker.coinbase = coinbase
//	self.coinbase = coinbase
//	self.worker.Stack = stack
//	//	glog.V(logger.Info).Infoln(" stack is stack")
//	//	self.worker.Resetworker()
//	if atomic.LoadInt32(&self.canStart) == 0 {
//		glog.V(logger.Info).Infoln("Can not start mining operation due to network sync (starts when finished)")
//		return
//	}

//	atomic.StoreInt32(&self.mining, 1)

//	for i := 0; i < threads; i++ {
//		self.worker.register(NewCpuAgent(i, self.pow))
//	}

//	glog.V(logger.Info).Infof("Starting mining operation (CPU=%d TOT=%d)\n", threads, len(self.worker.agents))

//	self.worker.start()

//	self.worker.commitNewWork()
//}

//func (self *Miner) Stop() {
//	self.worker.stop()

//FIXME 为不影响backend调用，保留空函数
func (self *Miner) Stop() {
}

//func (self *Miner) Register(agent Agent) {
//	if self.Mining() {
//		agent.Start()
//	}
//	self.worker.register(agent)
//}

//func (self *Miner) Unregister(agent Agent) {
//	self.worker.unregister(agent)
//}

//func (self *Miner) Mining() bool {
//	return atomic.LoadInt32(&self.mining) > 0
//}
//FIXME 为了backend中调用，暂时保留
func (self *Miner) Mining() bool {
	return atomic.LoadInt32(&self.mining) > 0
}

/*
func (self *Miner) HashRate() (tot int64) {
	tot += self.pow.GetHashrate()
	// do we care this might race? is it worth we're rewriting some
	// aspects of the worker/locking up agents so we can get an accurate
	// hashrate?
	for agent := range self.worker.agents {
		tot += agent.GetHashRate()
	}
	return
}*/

func (self *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize.Uint64() {
		return fmt.Errorf("Extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}

	self.worker.extra = extra
	return nil
}

// Pending returns the currently pending block and associated state.
func (self *Miner) Pending() (*Block, *StateDB) {
	return self.worker.pending()
}

func (self *Miner) SetEtherbase(addr common.Address) {
	self.coinbase = addr
	self.worker.setEtherbase(addr)
}

func (self *Miner) GetWorker() *worker {
	return self.worker
}

func (self *Miner) AddPrePrepare(prepre PrePrepare) {
	glog.V(logger.Info).Infof("addpreprepare=%d", prepre)
	go self.mux.Post(prepre)
}

func (self *Miner) GetEth() Backend {
	return self.eth
}

func (self *Miner) GetCurLeader() (string, error) {
	lid, _ := self.worker.GetConStatus()
	if lid != "" {
		return lid, nil
	}
	return lid, errors.New("do not have current leader")
}

func (self *Miner) GetLeader() string {
	curleader, _ := self.worker.GetConStatus() // 获取当前主节点
	//glog.V(logger.Info).Infof("currentleader is %d, diconection node is %d", curleader, disInfo.NodeID)
	//if curleader == disInfo.NodeID {
	//	glog.V(logger.Info).Infof("leader is down")
	//	//go self.worker.SendViewchange()
	//}
	return curleader
}
