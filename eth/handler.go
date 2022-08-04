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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	// "github.com/DWBC-ConPeer/core/VRS"
	// "github.com/DWBC-ConPeer/core/dencom"
	// "github.com/DWBC-ConPeer/core/types/HeaderWithVRS"
	"golang.org/x/net/context"

	"github.com/DWBC-ConPeer/common"
	"github.com/DWBC-ConPeer/event"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/rlp"

	// "github.com/DWBC-ConPeer/core"
	// "github.com/DWBC-ConPeer/core/types/BlockDM"
	// "github.com/DWBC-ConPeer/core/types/HeaderDM"
	// "github.com/DWBC-ConPeer/core/types/ReceiptDM"
	// "github.com/DWBC-ConPeer/core/types/TXDM"
	// "github.com/DWBC-ConPeer/eth/downloader"
	// "github.com/DWBC-ConPeer/eth/fetcher"
	// "github.com/DWBC-ConPeer/ethdb"
	// "github.com/DWBC-ConPeer/event"
	// "github.com/DWBC-ConPeer/logger"
	// "github.com/DWBC-ConPeer/logger/glog"
	// "github.com/DWBC-ConPeer/miner"
	"github.com/DWBC-ConPeer/p2p"
	"github.com/DWBC-ConPeer/p2p/discover"
	"github.com/DWBC-ConPeer/pns"
	// "github.com/DWBC-ConPeer/pow"
)

const (
	softResponseLimit = 2 * 1024 * 1024 // Target maximum size of returned blocks, headers or node data.
	estHeaderRlpSize  = 500             // Approximate size of an RLP encoded block header
)

var (
	daoChallengeTimeout = 15 * time.Second // Time allowance for a node to reply to the DAO handshake challenge
)

// errIncompatibleConfig is returned if the requested protocols and configs are
// not compatible (low protocol version restrictions and high requirements).
var errIncompatibleConfig = errors.New("incompatible configuration")

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

type ProtocolManager struct {
	networkId int

	// fastSync uint32 // Flag whether fast sync is enabled (gets disabled if we already have blocks)
	// synced   uint32 // Flag whether we're considered synchronised (enables transaction processing)

	// txpool     txPool
	// blockchain *core.BlockChain
	// miner      *miner.Miner // 添加miner属性
	// //chaindb     ethdb.Database
	// dbs         map[string]ethdb.Database
	// chainconfig *core.ChainConfig

	// downloader *downloader.Downloader
	// fetcher    *fetcher.Fetcher
	Peers *peerSet

	SubProtocols []p2p.Protocol

	eventMux *event.TypeMux
	MsgSub   event.Subscription
	Consm    event.Subscription
	Checkp   event.Subscription
	Zhuanfa  event.Subscription
	// txSub         event.Subscription
	// minedBlockSub event.Subscription
	// pbftSub       event.Subscription // add a new  Sub to subscribe pbft message
	// preSub        event.Subscription // add a new Sub to subscribe prepare message
	// commitSub     event.Subscription // add a new Sub to subscribe commit message
	// checkpointSub event.Subscription // add a new Sub to subscribe checkpoint message
	// newviewSub    event.Subscription // add a new Sub to subscribe newview message
	// viewchangeSub event.Subscription // add a new Sub to subscribe viewchange message

	// AccountStatusRequestSub event.Subscription // 移动设备消息
	// AccountStatusReplySub   event.Subscription

	// conMsgSub event.Subscription // conMsg subscription

	// pbftMsgSub event.Subscription // pbftMsg subscription

	// // channels for fetcher, syncer, txsyncLoop
	newPeerCh chan *peer
	// txsyncCh    chan *txsync
	quitSync chan struct{}
	// noMorePeers chan struct{}

	// // wait group is used for graceful shutdowns during downloading
	// // and processing
	wg sync.WaitGroup

	// badBlockReportingEnabled bool

	// pbfteventMux *event.TypeMux
	NodeType  uint64 // 固化数据类型，共识节点恒为1
	PNService *pns.PermissionedNodeService
}

// func (self *ProtocolManager) GetPbftMux() *event.TypeMux {
// 	return self.pbfteventMux
// }

// NewProtocolManager returns a new ethereum sub protocol manager. The Ethereum sub protocol manages peers capable
// with the ethereum network.
// add a param pnService *pns.PermissionedNodeService
func NewProtocolManager(networkId int, eventMux *event.TypeMux, PnService *pns.PermissionedNodeService) (*ProtocolManager, error) {
	// Create the protocol manager with the base fields
	manager := &ProtocolManager{
		eventMux:  eventMux,
		networkId: networkId,
		Peers:     newPeerSet(),
		newPeerCh: make(chan *peer),
		quitSync:  make(chan struct{}),
		NodeType:  uint64(1),
		PNService: PnService,
	}
	// Initiate a sub-protocol for every implemented version we can handle
	manager.SubProtocols = make([]p2p.Protocol, 0, len(ProtocolVersions))
	for i, version := range ProtocolVersions {
		// Skip protocol version if incompatible with the mode of operation
		if version < eth63 {
			continue
		}
		// Compatible; initialise the sub-protocol
		version := version // Closure for the run
		manager.SubProtocols = append(manager.SubProtocols, p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  ProtocolLengths[i],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				//				glog.V(logger.Info).Infoln("seriously don't know how it work= %d", p)
				peer := manager.newPeer(int(version), p, rw)
				select {
				default:
					// <-manager.newPeerCh
					// fmt.Println("准备注册新节点") 测试用
					err := manager.Peers.Register(peer) //注册新的节点
					if err != nil {
						panic(err.Error())
					}
					manager.wg.Add(1)
					defer manager.wg.Done()
					defer manager.removePeer(peer.id) //删除节点
					// peer = <-manager.newPeerCh //把peer取出来
					return manager.handle(peer)
				case <-manager.quitSync:
					return p2p.DiscQuitting
				}
			},
			NodeInfo: func() interface{} {
				return manager.NodeInfo()
			},
			PeerInfo: func(id discover.NodeID) interface{} {
				if p := manager.Peers.Peer(fmt.Sprintf("%x", id[:8])); p != nil {
					return p.Info()
				}
				return nil
			},
		})
	}
	if len(manager.SubProtocols) == 0 {
		return nil, errIncompatibleConfig
	}
	return manager, nil
}

func (pm *ProtocolManager) removePeer(id string) {
	// // Short circuit if the peer was already removed
	peer := pm.Peers.Peer(id)
	if peer == nil {
		return
	}
	glog.V(logger.Debug).Infoln("Removing peer", id)

	// // Unregister the peer from the downloader and Ethereum peer set
	// //	if peer.NodeType == uint64(1) {
	// pm.downloader.UnregisterPeer(id)
	if err := pm.Peers.Unregister(id); err != nil {
		glog.V(logger.Error).Infoln("Removal failed:", err)
	}
	//	}

	// // Hard disconnect at the networking layer
	// // FIXME fix this bug
	// if peer != nil /*&& peer.NodeType == uint64(1)*/ { // 不去除移动设备节点
	// 	peer.Peer.Disconnect(p2p.DiscUselessPeer)
	// }
}

func (pm *ProtocolManager) Start() {
	pm.MsgSub = pm.eventMux.Subscribe(TestMsg{})
	pm.Checkp = pm.eventMux.Subscribe(Checkpoint{})
	pm.Consm = pm.eventMux.Subscribe(ConsensusMsg{})
	pm.Zhuanfa = pm.eventMux.Subscribe(ConsensusMsg2{})
	go pm.BroadcastLoop()
	go pm.ConBroadcastLoop()
	go pm.CheckpointBroadcastLoop()
	go pm.ZhuanfaBroadcastLoop()
}

func (pm *ProtocolManager) BroadcastLoop() { //编写的测试通信
	for obj := range pm.MsgSub.Chan() {
		ev := obj.Data.(TestMsg)
		// fmt.Printf("TestMsg: %v\n", ev)
		for _, peer := range pm.Peers.GetPeers() {
			if pm.PNService.JudgeConPermissioned(peer.id) {
				peer.SendTestSendMsg(&ev, pm.PNService.NodePrk)
			}
		}
	}
}
func (pm *ProtocolManager) ConBroadcastLoop() { //编写的测试通信
	for obj := range pm.Consm.Chan() {
		ev := obj.Data.(ConsensusMsg)
		// fmt.Printf("TestMsg: %v\n", ev)
		for _, peer := range pm.Peers.GetPeers() {
			if pm.PNService.JudgeConPermissioned(peer.id) {
				//FormatOutput(peer.Peer.LocalAddr().String(), peer.Peer.RemoteAddr().String(), ev.Type)
				FormatOutput(Node1.nodeID, Returnadd(peer.id), ev.Type)
				peer.SendConsSendMsg(&ev, pm.PNService.NodePrk)
			}
		}
	}
}
func (pm *ProtocolManager) CheckpointBroadcastLoop() { //编写的测试通信
	for obj := range pm.Checkp.Chan() {
		ev := obj.Data.(Checkpoint)
		// fmt.Printf("TestMsg: %v\n", ev)
		for _, peer := range pm.Peers.GetPeers() {
			if pm.PNService.JudgeConPermissioned(peer.id) {
				//FormatOutput(peer.Peer.LocalAddr().String(), peer.Peer.RemoteAddr().String(), "checkpoint")
				FormatOutput(Node1.nodeID, Returnadd(peer.id), "checkpoint")
				peer.SendCheckPointMsg(&ev, pm.PNService.NodePrk)
			}
		}
	}
}
func (pm *ProtocolManager) ZhuanfaBroadcastLoop() { //编写的测试通信
	for obj := range pm.Zhuanfa.Chan() {
		ev := obj.Data.(ConsensusMsg2)
		// fmt.Printf("TestMsg: %v\n", ev)
		for _, peer := range pm.Peers.GetPeers() {
			if pm.PNService.JudgeConPermissioned(peer.id) {
				//FormatOutput(peer.Peer.LocalAddr().String(), peer.Peer.RemoteAddr().String(), ev.Newsender+"转发的"+ev.Sender+"的"+ev.Type)
				FormatOutput(ev.Newsender, Returnadd(peer.id), ev.Newsender+"转发的"+ev.Sender+"的"+ev.Type)
				peer.SendZhuanfaMsg(&ev, pm.PNService.NodePrk)
			}
		}
	}
}
func (pm *ProtocolManager) Stop() {
	// glog.V(logger.Info).Infoln("Stopping ethereum protocol handler...")
	// pm.txSub.Unsubscribe()         // quits txBroadcastLoop
	// pm.minedBlockSub.Unsubscribe() // quits blockBroadcastLoop
	pm.MsgSub.Unsubscribe() // quits requestBroadcastLoop
	pm.Consm.Unsubscribe()
	pm.Checkp.Unsubscribe()
	pm.Zhuanfa.Unsubscribe()
	//改：上面加了两个
	// // Quit the sync loop.
	// // After this send has completed, no new peers will be accepted.
	// pm.noMorePeers <- struct{}{}
	// // Quit fetcher, txsyncLoop.
	close(pm.quitSync)
	// Disconnect existing sessions.
	// This also closes the gate for any new registrations on the peer set.
	// sessions which are already established but not added to pm.peers yet
	// will exit when they try to register.
	pm.Peers.Close()
	// Wait for all peer handler goroutines and the loops to come down.
	pm.wg.Wait()
	// glog.V(logger.Info).Infoln("Ethereum protocol handler stopped")
}

func (pm *ProtocolManager) newPeer(pv int, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {

	id := p.ID()
	pk, err := pm.PNService.GetPKByPID(fmt.Sprintf("%x", id[:]))
	if err != nil {
		glog.V(logger.Info).Infoln("get nodePublickey by nodeID err =%d ", err)
	}
	decodeBytes, err := base64.StdEncoding.DecodeString(pk)
	if err != nil {
		glog.V(logger.Info).Infoln("decode string pk to byte err =%d ", err)
	}
	p.NodePK = decodeBytes
	return newPeer(pv, p, newMeteredMsgWriter(rw))
}

// handle is the callback invoked to manage the life cycle of an eth peer. When
// this function terminates, the peer is disconnected. handle 是调用来管理 eth 对等点的生命周期的回调。 当此函数终止时，对端断开连接
func (pm *ProtocolManager) handle(p *peer) error {
	// main loop. handle incoming messages.
	var Err error
	resChan := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	go func(resChan chan error, ctx context.Context) {
		for {
			select {
			case Err = <-resChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}(resChan, ctx)

	for {
		if Err != nil {
			cancel()
			go closeChan(resChan)
			glog.V(logger.Debug).Infof(" %v:message handling failed: %v", p, Err)
			return Err
		} else {
			err := pm.handleMsg(p, resChan, ctx)
			if err != nil {
				cancel()
				go closeChan(resChan)
				log.Printf(" message handling failed: %v", err.Error())
				return err
			}
		}
	}
}

func closeChan(resChan chan error) {
	for {
		select {
		case <-resChan: //把阻塞的消息消费了
		default:
			close(resChan)
			return
		}
	}
}

//将消息处理结果返回给通信地址
//只返回错误，空则不发送
func sendHandleMsgRes(resChan chan error, ctx context.Context, err error) {
	if err != nil {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("sendHandleMsgRes catch panic: %v ", r)
				glog.V(logger.Info).Info("!!!!!!!!!", err)
			}
		}()
		t1 := time.NewTimer(time.Millisecond * 1)
		defer t1.Stop()
		//for{
		select {
		case <-ctx.Done():
			return
		case <-t1.C:
			resChan <- err
			return
		}
		//}
	}

}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (pm *ProtocolManager) handleMsg(p *peer, resChan chan error, ctx context.Context) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()

	if err != nil {
		glog.V(logger.Info).Infoln("handleMsg err =%d ", err)
		// send a Event to worker
		// go pm.pbfteventMux.Post(core.PeerDisCon{NodeID: p.ID().String()})
		return err
	}
	if msg.Size > ProtocolMaxMsgSize {
		glog.V(logger.Debug).Infoln("handleMsgErr")
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()
	//	glog.V(logger.Info).Infoln("msg code ", msg.Code)

	// // Handle the message depending on its contents
	switch {
	case msg.Code == TestSendMsg:
		var cd CommunicationInfo
		if err := msg.Decode(&cd); err != nil {
			glog.V(logger.Debug).Infoln("handleMsgErr")
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		go func(pm *ProtocolManager, p *peer, msg p2p.Msg, cd CommunicationInfo, resChan chan error, ctx context.Context) error {
			err := func() error {

				// 使用节点公钥验签
				if flag, err := ValidateSig(p.NodePK, cd, false); flag == false {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrIllegalSig, "msg %v: %v", msg, err)
				}
				var testSendMsg TestMsg
				// fmt.Printf("cd.PayLoad: %v\n", cd.PayLoad)测试用
				err = rlp.DecodeBytes(cd.PayLoad, &testSendMsg)
				if err != nil {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrDecode, "msg %v: %v", msg, err)
				}
				fmt.Printf("testSendMsg: %v\n", testSendMsg)
				var TestReciveMsg = TestMsg{
					Msg:    "TestReciveMsg",
					Sender: "Node2", // 将公钥转为base64
					Time:   time.Now().Format("2006/01/02 15:04:05"),
				}
				for _, peer := range pm.Peers.GetPeers() {
					if pm.PNService.JudgeConPermissioned(peer.id) {
						peer.SendTestReciveMsg(&TestReciveMsg, pm.PNService.NodePrk)
					}
				}

				// according to conmsg.Type to transfet struct

				return nil
			}()
			sendHandleMsgRes(resChan, ctx, err)
			return err
		}(pm, p, msg, cd, resChan, ctx)
	case msg.Code == TestReciveMsg:
		var cd CommunicationInfo
		if err := msg.Decode(&cd); err != nil {
			glog.V(logger.Debug).Infoln("handleMsgErr")
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		go func(pm *ProtocolManager, p *peer, msg p2p.Msg, cd CommunicationInfo, resChan chan error, ctx context.Context) error {
			err := func() error {

				// 使用节点公钥验签
				if flag, err := ValidateSig(p.NodePK, cd, false); flag == false {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrIllegalSig, "msg %v: %v", msg, err)
				}
				var testReciveMsg TestMsg
				err = rlp.DecodeBytes(cd.PayLoad, &testReciveMsg)
				if err != nil {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrDecode, "msg %v: %v", msg, err)
				}
				fmt.Printf("testReciveMsg: %v\n", testReciveMsg)
				// according to conmsg.Type to transfet struct
				return nil
			}()
			sendHandleMsgRes(resChan, ctx, err)
			return err
		}(pm, p, msg, cd, resChan, ctx)
	case msg.Code == ConMsg:
		var cd CommunicationInfo
		if err := msg.Decode(&cd); err != nil {
			glog.V(logger.Debug).Infoln("handleMsgErr")
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		go func(pm *ProtocolManager, p *peer, msg p2p.Msg, cd CommunicationInfo, resChan chan error, ctx context.Context) error {
			err := func() error {

				// 使用节点公钥验签
				if flag, err := ValidateSig(p.NodePK, cd, false); flag == false {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrIllegalSig, "msg %v: %v", msg, err)
				}
				var testReciveMsg ConsensusMsg
				err = rlp.DecodeBytes(cd.PayLoad, &testReciveMsg)
				if err != nil {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrDecode, "msg %v: %v", msg, err)
				}
				if testReciveMsg.Type == "prepare" {
					var prepare = Prepare{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &prepare)
					if errr != nil {
						fmt.Println("转换错误")
					}
					//fmt.Printf("prepare: %v\n", testReciveMsg)
					fmt.Printf("转换结果: %v\n", prepare)
				}
				if testReciveMsg.Type == "commit" {
					fmt.Printf("commit: %v\n", testReciveMsg)
				}
				if testReciveMsg.Type == "PBFTRequest" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Type)
					FormatOutput(testReciveMsg.Sender, Node1.nodeID, testReciveMsg.Type)
					req := Request{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvRequest(&req)
				}
				if testReciveMsg.Type == "PBFTPrePrepare" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Type)
					FormatOutput(testReciveMsg.Sender, Node1.nodeID, testReciveMsg.Type)
					req := PrePrepare{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvPrePrepare(&req)
				}
				if testReciveMsg.Type == "PBFTPrepare" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Type)
					FormatOutput(testReciveMsg.Sender, Node1.nodeID, testReciveMsg.Type)
					req := Prepare{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvPrepare(&req)
				}
				if testReciveMsg.Type == "PBFTCommit" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Type)
					FormatOutput(testReciveMsg.Sender, Node1.nodeID, testReciveMsg.Type)
					req := Commit{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvCommit(&req)
				}
				// according to conmsg.Type to transfet struct
				return nil
			}()
			sendHandleMsgRes(resChan, ctx, err)
			return err
		}(pm, p, msg, cd, resChan, ctx)
	case msg.Code == Checkpoin:
		var cd CommunicationInfo
		if err := msg.Decode(&cd); err != nil {
			glog.V(logger.Debug).Infoln("handleMsgErr")
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		go func(pm *ProtocolManager, p *peer, msg p2p.Msg, cd CommunicationInfo, resChan chan error, ctx context.Context) error {
			err := func() error {

				// 使用节点公钥验签
				if flag, err := ValidateSig(p.NodePK, cd, false); flag == false {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrIllegalSig, "msg %v: %v", msg, err)
				}
				var testReciveMsg Checkpoint
				err = rlp.DecodeBytes(cd.PayLoad, &testReciveMsg)
				if err != nil {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrDecode, "msg %v: %v", msg, err)
				}
				//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), "checkpoint")
				FormatOutput(testReciveMsg.Sender, Node1.nodeID, "checkpoint")
				Node1.RecvCheckpoint(&testReciveMsg)
				fmt.Printf("checkpointMsg: %v\n", testReciveMsg)
				// according to conmsg.Type to transfet struct
				return nil
			}()
			sendHandleMsgRes(resChan, ctx, err)
			return err
		}(pm, p, msg, cd, resChan, ctx)
	case msg.Code == Zhuanfa:
		var cd CommunicationInfo
		if err := msg.Decode(&cd); err != nil {
			glog.V(logger.Debug).Infoln("handleMsgErr")
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		go func(pm *ProtocolManager, p *peer, msg p2p.Msg, cd CommunicationInfo, resChan chan error, ctx context.Context) error {
			err := func() error {

				// 使用节点公钥验签
				if flag, err := ValidateSig(p.NodePK, cd, false); flag == false {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrIllegalSig, "msg %v: %v", msg, err)
				}
				var testReciveMsg ConsensusMsg2
				err = rlp.DecodeBytes(cd.PayLoad, &testReciveMsg)
				if err != nil {
					glog.V(logger.Debug).Infoln("handleMsgErr")
					return errResp(ErrDecode, "msg %v: %v", msg, err)
				}
				if testReciveMsg.Type == "PBFTRequest" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					FormatOutput(testReciveMsg.Newsender, Node1.nodeID, testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					req := Request{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvRequest(&req)
				}
				if testReciveMsg.Type == "PBFTPrePrepare" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					FormatOutput(testReciveMsg.Newsender, Node1.nodeID, testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					req := PrePrepare{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvPrePrepare(&req)
				}
				if testReciveMsg.Type == "PBFTPrepare" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					FormatOutput(testReciveMsg.Newsender, Node1.nodeID, testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					req := Prepare{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvPrepare(&req)
				}
				if testReciveMsg.Type == "PBFTCommit" {
					//FormatOutput(p.Peer.RemoteAddr().String(), p.Peer.LocalAddr().String(), testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					FormatOutput(testReciveMsg.Newsender, Node1.nodeID, testReciveMsg.Newsender+"转发的"+testReciveMsg.Sender+"的"+testReciveMsg.Type)
					req := Commit{}
					errr := json.Unmarshal([]byte(testReciveMsg.Msg), &req)
					if errr != nil {
						fmt.Println("转换错误")
					}
					Node1.RecvCommit(&req)
				}
				// according to conmsg.Type to transfet struct
				return nil
			}()
			sendHandleMsgRes(resChan, ctx, err)
			return err
		}(pm, p, msg, cd, resChan, ctx)
	default:
		glog.V(logger.Debug).Infoln("handleMsgErr")
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	return nil
}

// EthNodeInfo represents a short summary of the Ethereum sub-protocol metadata known
// about the host peer.
type EthNodeInfo struct {
	Network    int         `json:"network"`    // Ethereum network ID (0=Olympic, 1=Frontier, 2=Morden)
	Difficulty *big.Int    `json:"difficulty"` // Total difficulty of the host's blockchain
	Genesis    common.Hash `json:"genesis"`    // SHA3 hash of the host's genesis block
	Head       common.Hash `json:"head"`       // SHA3 hash of the host's best owned block
}

// NodeInfo retrieves some protocol metadata about the running host node.
func (self *ProtocolManager) NodeInfo() *EthNodeInfo {
	// return &EthNodeInfo{
	// 	Network:    self.networkId,
	// 	Difficulty: self.blockchain.GetTd(self.blockchain.CurrentBlock().Hash()),
	// 	Genesis:    self.blockchain.Genesis().Hash(),
	// 	Head:       self.blockchain.CurrentBlock().Hash(),
	// }
	return nil
}
