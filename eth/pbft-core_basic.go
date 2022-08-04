package eth

import (
	//"encoding/json"
	"errors"
	"fmt"

	//"github.com/DWBC-ConPeer/core"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"

	//	crypto "github.com/DWBC-ConPeer/crypto-gm"

	"sync/atomic"
	"time"
	//"github.com/DWBC-ConPeer/common"
	//"github.com/DWBC-ConPeer/core/types/BlockDM"
)

//改：
type BlockForceSync struct {
}
type ConStatusRequest struct {
	Timestamp time.Time
	Sender    string
}

type ConsensusRequest struct {
	Timestamp time.Time
	Sender    string
}

func (self *worker) SyncConStatus() {
	//	CRwaitGroup.Wait()
	if true { //self.CalActivePNode() /*&& !self.Ready() && !self.getElectionFlag()*/ { // 2/3的节点已启动且已相互连通
		//CRwaitGroup.Add(1)
		//self.GetNetworkConstatus_syncView()
		//改：
		cq := ConStatusRequest{
			Timestamp: time.Now(),
			Sender:    self.GetSelfNodeID(),
		}
		cmsg := ConsensusMsg{
			Msg:       self.Tostring(cq),
			Timestamp: time.Now(),
			Type:      "ConStatusRequest",
			Sender:    self.GetSelfNodeID(),
		}
		go self.PbftMux.Post(cmsg) // send cq to other nodes
	}
}
func (self *worker) ISLeader() bool {
	lid, _ := self.GetConStatus()
	return lid == self.GetSelfNodeID()
}
func (self *worker) ResetCommitStore() {
	commitMux.Lock()
	commitStore = make(map[uint64]string)
	commitMux.Unlock()
}

//更新执行过checkPoint的最大区块高度，比当前大才更新
func updateCheckpointBH(bh uint64) {
	chkMux.Lock()
	defer chkMux.Unlock()
	if checkpointBH < bh {
		checkpointBH = bh
	}
}

func getCheckpointBH() uint64 {
	chkMux.RLock()
	defer chkMux.RUnlock()
	return checkpointBH
}

func (self *worker) GetActiveView() bool {
	self.viewmux.RLock()
	flag := self.activeView
	self.viewmux.RUnlock()
	return flag
}

func (self *worker) SetActiveView(flag bool) {
	self.viewmux.Lock()
	self.activeView = flag
	self.viewmux.Unlock()
	glog.V(logger.Debug).Infof("set activeview %d", flag)
}

//更新租约更换计时器的开始时间，区块recvcheckpoint，主节点更新时会更新
func updateLeaseBeat() {
	lbMux.Lock()
	leaseBeat = time.Now()
	lbMux.Unlock()
}

func getLeaseBeat() time.Time {
	lbMux.RLock()
	defer lbMux.RUnlock()
	return leaseBeat
}

/*
** 获取 Quorum 数量
 */
func (instance *worker) getQuorum() int {
	instance.quroummux.RLock()
	defer instance.quroummux.RUnlock()

	currentCSNum := instance.N

	if currentCSNum == 0 || currentCSNum < 2 { // 当没有任何许可节点时，不发起任何交易
		return 10000000
	}

	if currentCSNum == 2 {
		return currentCSNum
	} else if currentCSNum == 3 {
		return currentCSNum - 1
	} else {
		f := currentCSNum / 3
		return currentCSNum - f
	}
}

func (instance *worker) getCurrentCSNum() int {
	instance.quroummux.RLock()
	defer instance.quroummux.RUnlock()
	return instance.N
}

func (instance *worker) getElectionQuorum() int {
	instance.quroummux.RLock()
	defer instance.quroummux.RUnlock()

	currentCSNum := instance.N

	if currentCSNum == 0 || currentCSNum < 2 { // 当没有任何许可节点时，不发起任何交易
		return 10000000
	}

	if currentCSNum == 2 {
		return currentCSNum
	} else if currentCSNum == 3 {
		return currentCSNum - 1
	} else {
		f := currentCSNum / 2
		return f + 1
	}
}

func (instance *worker) sendConMsg() {

}

//下面自己改的
func (instance *worker) sendConMsg_transfer1(msg *PrePrepare, msgtype string) {
	//只转发sender不是自己的
	if msg.GetSender() != instance.GetSelfNodeID() {
		var msg_str string
		if msgtype == "PBFTRequest" || msgtype == "PBFTPrePrepare" || msgtype == "PBFTPrepare" {
			msg_str = instance.Tostring(*msg)
		} else {
			msg_str = instance.Tostring(*msg)
		}
		cmsg_transfer := ConsensusMsg2{
			Msg:       msg_str,
			Timestamp: time.Now(),
			Type:      msgtype,
			Sender:    msg.GetSender(),
			Newsender: instance.nodeID,
			Other:     "转发消息",
		}
		var temp2 = LogMsg{
			Sender:  instance.GetSelfNodeID(),
			Time:    time.Now(),
			Content: instance.Tostring(msg),
			Other:   "转发的preprepare消息",
		}
		Writelog("LogInfo/sendlog.txt", temp2)
		go instance.PbftMux.Post(cmsg_transfer)
	}
}
func (instance *PrePrepare) GetSender() string {
	return instance.Sender
}
func (instance *worker) sendConMsg_transfer2(msg *Prepare, msgtype string) {
	//只转发sender不是自己的
	if msg.GetSender() != instance.GetSelfNodeID() {
		var msg_str string
		if msgtype == "PBFTRequest" || msgtype == "PBFTPrePrepare" || msgtype == "PBFTPrepare" {
			msg_str = instance.Tostring(*msg)
		} else {
			msg_str = instance.Tostring(*msg)
		}
		cmsg_transfer := ConsensusMsg2{
			Msg:       msg_str,
			Timestamp: time.Now(),
			Type:      msgtype,
			Sender:    msg.GetSender(),
			Newsender: instance.nodeID,
			Other:     "转发消息",
		}
		var temp2 = LogMsg{
			Sender:  instance.GetSelfNodeID(),
			Time:    time.Now(),
			Content: instance.Tostring(msg),
			Other:   "转发的prepare消息",
		}
		Writelog("LogInfo/sendlog.txt", temp2)
		go instance.PbftMux.Post(cmsg_transfer)
	}
}
func (instance *Prepare) GetSender() string {
	return instance.Sender
}
func (instance *worker) sendConMsg_transfer3(msg *Commit, msgtype string) {
	//只转发sender不是自己的
	if msg.GetSender() != instance.GetSelfNodeID() {
		var msg_str string
		if msgtype == "PBFTRequest" || msgtype == "PBFTPrePrepare" || msgtype == "PBFTPrepare" {
			msg_str = instance.Tostring(*msg)
		} else {
			msg_str = instance.Tostring(*msg)
		}
		cmsg_transfer := ConsensusMsg2{
			Msg:       msg_str,
			Timestamp: time.Now(),
			Type:      msgtype,
			Sender:    msg.GetSender(),
			Newsender: instance.nodeID,
			Other:     "转发消息",
		}
		var temp2 = LogMsg{
			Sender:  instance.GetSelfNodeID(),
			Time:    time.Now(),
			Content: instance.Tostring(msg),
			Other:   "转发的commit消息",
		}
		Writelog("LogInfo/sendlog.txt", temp2)
		go instance.PbftMux.Post(cmsg_transfer)
	}
}
func (instance *Commit) GetSender() string {
	return instance.Sender
}

//上边自己改的

// 当节点不处于挖矿状态、与大多数节点建立链接，本节点是本轮的leader，
// activeview与ready是否为true，以及当前是否有正在共识的区块
// TODO ready 表示Lader Selection是否完成
// TODO activeview 表示当前是否有存在的共识
// TODO 对判断条件进行删减
// CanMining 判断当前是否可以构建区块
func (self *worker) CanMining() bool {
	if atomic.LoadInt32(&self.mining) == 1 {
		//glog.V(logger.Info).Infoln("====2======  ")

		return false
	}

	cu_BH := self.chain.CurrentBlock //().NumberU64()
	self.pvbAndBeatsMux.RLock()
	_, ok := self.possibleValidBlock[cu_BH+1]
	self.pvbAndBeatsMux.RUnlock()
	if ok {
		//glog.V(logger.Info).Infoln("====4==pvb has block:",cu_BH + 1)

		return false
	}

	if true /*self.CalActivePNode()*/ && self.ISLeader() &&
		/*self.GetActiveView() &&*/ self.Ready() {
		//glog.V(logger.Info).Infoln("====5=====can mining======  ")

		return true
	}
	//glog.V(logger.Info).Infoln("====6===act, lead, ready===  ", self.CalActivePNode(), self.ISLeader(), self.Ready())

	return false
}

/*采用新的hash，这个可以注释掉了
// hashReq 计算Request的哈希值
func hashReq(req *core.Request) (digest string) {
	//	return req.ReqID.Str()
	// 两种方式： 1 reqID+blockHeight
	//          2 hash
	dat, err := json.Marshal(req)
	if err != nil {
		return ""
	}
	//return crypto.Keccak256Hash(dat).Str()
	return crypto.Keccak256Hash(dat).Hex()
}
*/

//可能正确的block，第一个key表示区块高度，第二个key表示block的hash，第二个value表示具体的block
//possibleValidBlock sync.Map//map[uint64](map[common.Hash]*BlockDM.Block)
func (self *worker) putBlockOnPossibleValidBlock(block /*BlockDM.Block*/ *Block) {

	a := make(map[string]*Block)
	fmt.Println("2222222222222222222222222")
	blocks, ok := self.possibleValidBlock[block.Number]
	if ok {
		a = blocks
	}
	fmt.Println("33333333333333333333333333333")
	a[block.Hash()] = block
	fmt.Println("44444444444444444444")
	self.possibleValidBlock[block.Number] = a
	fmt.Println("55555555555555555555555")
}

func (instance *worker) GetBlockFromPossibleValidBlock(bn uint64, bhash string) *Block { //*BlockDM.Block {
	instance.pvbAndBeatsMux.RLock()
	defer instance.pvbAndBeatsMux.RUnlock()
	blocks, ok := instance.possibleValidBlock[bn]
	if !ok {
		return nil
	}
	block, ok := blocks[bhash]
	if !ok {
		return nil
	}
	return block
}

func (instance *worker) PutPVBAndBeats(block /*BlockDM.Block*/ *Block) {
	//加锁
	instance.pvbAndBeatsMux.Lock()
	defer instance.pvbAndBeatsMux.Unlock()
	fmt.Println("ceshi..............")
	instance.putBlockOnPossibleValidBlock(block) // 将区块放入区块池
	fmt.Println(888888888888)
	instance.addbeats(block.Hash())
}

func (instance *worker) DeletePVBAndBeats(bn uint64, bhash string) {
	//加锁
	instance.pvbAndBeatsMux.Lock()
	defer instance.pvbAndBeatsMux.Unlock()
	instance.deleteImpossibleValidBlock(bn)
	instance.deletbeats(bhash) //  删除blockbeats
}

func (instance *worker) DeletePVBAndBeatsByHash(bhash string) {
	//加锁
	instance.pvbAndBeatsMux.Lock()
	defer instance.pvbAndBeatsMux.Unlock()
	instance.deletePVBAndBeatsByHash(bhash)
}

func (instance *worker) deletePVBAndBeatsByHash(bhash string) {
	instance.deletbeats(bhash) // reset
	instance.deleteImpossibleValidBlockByHash(bhash)
}

func (instance *worker) GetBlockFromPVBlockByHash(hash string) (*Block, uint64) {
	instance.pvbAndBeatsMux.RLock()
	defer instance.pvbAndBeatsMux.RUnlock()
	for bn, blocks := range instance.possibleValidBlock {
		if block, ok := blocks[hash]; ok { //FIXME 需要判断非空吗
			return block, bn
		}
	}
	return nil, 0
}

//FIXME
func (instance *worker) deleteImpossibleValidBlock(dbn uint64) {
	for bn, blocks := range instance.possibleValidBlock {
		if bn <= dbn {
			//删除beats
			//删除block
			for hash, _ := range blocks {
				delete(instance.blockbeats, hash)
			}
			delete(instance.possibleValidBlock, bn)
		}
	}
}

func (instance *worker) deleteImpossibleValidBlockByHash(hash string) {
	for bn, blocks := range instance.possibleValidBlock {
		if _, ok := blocks[hash]; ok {
			delete(blocks, hash)
			if len(blocks) == 0 {
				delete(instance.possibleValidBlock, bn)
			}
			return
		}
	}
}

func (instance *worker) GetPossibleBHByHash(hash string) (uint64, error) {
	//加锁
	instance.pvbAndBeatsMux.RLock()
	defer instance.pvbAndBeatsMux.RUnlock()
	return instance.getPossibleBHByHash(hash)
}

func (instance *worker) getPossibleBHByHash(hash string) (uint64, error) {
	bn := uint64(0)
	for bn, blocks := range instance.possibleValidBlock {
		if _, ok := blocks[hash]; ok {
			return bn, nil
		}
	}
	return bn, errors.New("non such block")
}

func (self *worker) ResetPVBAndBeats() {
	self.pvbAndBeatsMux.Lock()
	defer self.pvbAndBeatsMux.Unlock()
	self.blockbeats = make(map[string]time.Time)
	self.possibleValidBlock = make(map[uint64]map[string]*Block)
}

//map[common.Hash]time.Time 记录区块被共识的时间
func (self *worker) addbeats(n string) {
	if _, ok := self.blockbeats[n]; !ok {
		self.blockbeats[n] = time.Now()
	}

}

func (self *worker) deletbeats(n string) {
	delete(self.blockbeats, n)
}

//判断view是否相等
func (instance *worker) inV(v uint64) bool {
	_, view := instance.GetConStatus()
	return view == v
}

func (self *worker) SyncView(view uint64) {
	//_, cv := self.GetConStatus()
	//if view > cv {
	//	go self.SyncConStatus() // update constatus
	//}
	fmt.Println("已同步view", view)
}

// JudgeSync judge self node whether need to sync constatus and blockchain
// FIXME any consensus msg will triger this function, will casue multi initialConStatus()
func (self *worker) JudgeSync(bn uint64) bool {
	selfBlockNum := self.chain.CurrentBlock                                       //().NumberU64()
	if selfBlockNum < bn-1 /*self.eth.BlockChain().CurrentBlock().NumberU64()*/ { // 说明本节点需要同步区块
		//改：
		fsync := BlockForceSync{}
		go self.PbftMux.Post(fsync)
		go self.SyncConStatus() // update constatus
		return false
	}
	return true
}

func (instance *worker) GetRequestFromStore(reqDigest string) *Request /*core.Request*/ {
	req, ok := instance.reqStore.Load(reqDigest)
	if !ok {
		return nil
	}
	return req.(*Request) //*core.Request)
}

// Given a digest/view/seq, is there an entry in the certLog?
// If so, return it. If not, create it.
func (instance *worker) getCert(v uint64, reqID uint64) (cert *msgCert) {
	//	idx := msgID{v, instance.Str(n)}
	idx := msgID{v, reqID}

	instance.certMux.RLock() // 加锁
	cert, ok := instance.certStore[idx]
	instance.certMux.RUnlock() // 解锁

	if ok {
		return
	}

	cert = &msgCert{}
	instance.certMux.Lock()
	instance.certStore[idx] = cert
	instance.certMux.Unlock()
	return
}

func (instance *worker) putPrePrepareInCert(cview uint64, reqID uint64, preprep /*core.PrePrepare*/ PrePrepare) {
	cert := instance.getCert(cview, reqID)
	instance.certMux.Lock()
	defer instance.certMux.Unlock()
	if cert != nil {
		cert.prePrepare = &preprep
	}
}

//~~~~~~~~~~~~~~~~~~~~~~~基础函数~~~~~~~~~~~~~~~~~~~~~~~~~

/*
** prePrepared 函数：判断 request 是否到达 PrePrepared 状态
 */
func (instance *worker) prePrepared(digest string, v uint64, reqID uint64) bool {

	_, mInLog := instance.reqStore.Load(digest)

	if digest != "" && !mInLog {
		//glog.V(logger.Info).Infoln("prePrepared false by reqStore no such digest",v,reqID)
		return false
	}

	instance.certMux.RLock()         // 加锁
	defer instance.certMux.RUnlock() //解锁
	cert := instance.certStore[msgID{v, reqID}]
	if cert != nil {
		p := cert.prePrepare
		if p != nil && p.View == v && p.ReqID == reqID && p.RequestDigest == digest {
			return true
		}
	}
	glog.V(logger.Debug).Infoln("not have pre-prepared with view=%d, reqid=%d", v, reqID)
	return false
}

/*
** prepared 函数：判断 request 是否到达 prepared 状态
 */
func (instance *worker) prepared(digest string, v uint64, reqID uint64) bool {
	if !instance.prePrepared(digest, v, reqID) {
		//glog.V(logger.Info).Infoln("prepared false by not prePrepared",v,reqID)
		return false
	}

	quorum := 0
	instance.certMux.RLock()
	defer instance.certMux.RUnlock()
	cert := instance.certStore[msgID{v, reqID}]
	if cert == nil {
		return false
	}

	for _, p := range cert.prepare {
		if p.View == v && p.ReqID == reqID && p.RequestDigest == digest {
			quorum++
		}
	}

	if !(quorum >= instance.getQuorum()) {
		//glog.V(logger.Info).Infoln("prepared false by quorum less",v,reqID)
	}
	return quorum >= instance.getQuorum()
}

/*
** committed 函数：判断 request 是否到达 committed 状态
 */
func (instance *worker) committed(digest string, v uint64, reqID uint64) bool {
	if !instance.prepared(digest, v, reqID) {
		glog.V(logger.Info).Infoln("committed false by not prepared", v, reqID)
		return false
	}

	quorum := 0
	instance.certMux.RLock()
	defer instance.certMux.RUnlock()
	cert := instance.certStore[msgID{v, reqID}]

	if cert == nil {
		glog.V(logger.Info).Infoln("committed false by cert nil", v, reqID)
		return false
	}

	var senderCount = make(map[string]int)
	for _, p := range cert.commit {
		if p.View == v && p.ReqID == reqID {
			if _, ok := senderCount[p.Sender]; !ok { //ok，则不计数，不添加，防止巧合重复添加
				senderCount[p.Sender] = 0
				quorum++
			}
		}
	}
	if !(quorum >= instance.getQuorum()) {
		glog.V(logger.Debug).Infof("committed false: view %d reqID %d recvquorum %d quorum %d", v, reqID, quorum, instance.getQuorum())
	}
	return quorum >= instance.getQuorum()
}

/*
*  GetValidResFromCommit 函数，从commit中统计出 每个tx的投票数量
*  只有达到2/3的tx才被认为共识通过，这里的2/3是是指所有共识节点的2/3
 */

func (instance *worker) GetValidResFromCommit(cert *msgCert) int {
	instance.certMux.RLock() // 加锁
	poss := 0
	neg := 0
	all := 0
	var senderCount = make(map[string]int)
	for _, com := range cert.commit {
		if _, ok := senderCount[com.Sender]; !ok { //防止重复情况
			senderCount[com.Sender] = 0
			all++
			if com.VR.Opinion {
				poss++
			} else {
				neg++
			}
		}
	}
	instance.certMux.RUnlock()
	currentCSNum := instance.getCurrentCSNum() //当前许可节点总数
	future := currentCSNum - all               //未来可能收到的commit数

	quorum := instance.getQuorum() //阈值
	if poss >= quorum {            //积极达到阈值
		return possitive
	}
	if poss+future >= quorum { //未来可能积极
		glog.V(logger.Info).Infof("waiting poss commit recvAll=%d poss=%d neg=%d future=%d cPNodes=%d quorum=%d", all, poss, neg, future, currentCSNum, quorum)
		return futurePossitive
	}

	glog.V(logger.Info).Infof("Negative recvAll=%d poss=%d neg=%d future=%d cPNodes=%d quorum=%d", all, poss, neg, future, currentCSNum, quorum)
	return negative // 消极
}

//改：好像也不用了这个函数，先注释了吧
//获取达成一致的交易依赖图
func (instance *worker) getConsensusTxExes(digest string, v uint64, reqID uint64) ([]byte, error) {
	instance.certMux.RLock()
	defer instance.certMux.RUnlock()
	cert := instance.certStore[msgID{v, reqID}]
	if cert == nil {
		return nil, errors.New("cert is nil")
	}

	var exe []byte
	var count int
	txExeCounts := make(map[string]int) //<prepare数组下标,对应票数>
	txExe := make(map[string][]byte)

	for _, p := range cert.prepare {
		if p.View == v && p.ReqID == reqID && p.RequestDigest == digest {
			//判空，即使是空区块，一个空的交易依赖图，转成byte也不应该是空，如果是nil， 不认可
			if len(p.TxsExeSeqs) != 0 {
				//hash := crypto.Keccak256Hash(p.TxsExeSeqs).Hex()
				txExe[string(p.TxsExeSeqs)] = p.TxsExeSeqs
				txExeCounts[string(p.TxsExeSeqs)]++
			}
		}
	}

	for exhash, c := range txExeCounts {
		if c > count {
			count = c
			exe = txExe[exhash]
		}
	}

	if !(count >= instance.getQuorum()) {
		//glog.V(logger.Info).Infoln("prepared false by quorum less",v,reqID)
		return nil, errors.New("not have consensus txdepend reached quorumNum")
	}
	return exe, nil
}

func (instance *worker) ResetConsensusData() {

	instance.certMux.Lock()
	for k, _ := range instance.certStore {
		delete(instance.certStore, k)
	}
	instance.certMux.Unlock()
	instance.reqStore.Range(func(k, v interface{}) bool {
		instance.reqStore.Delete(k)
		return true
	})

}
