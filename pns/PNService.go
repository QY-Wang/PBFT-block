package pns

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"

	"github.com/DWBC-ConPeer/crypto-gm/sm2"

	"errors"

	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
)

// 许可节点管理对象
type PermissionedNodeService struct {
	pNodes    map[string]PNode //节点列表，key为节点enode
	pnodesMux sync.RWMutex     // pNodes 的读写锁
	NodePrk   *sm2.PrivateKey  //节点私钥 //有地方使用
}

// PNode 许可节点信息(为其他模块提供支持)
type PNode struct {
	ID   string //节点enode
	Type string //节点类型，TXNode或者ConNode
	PK   string //节点公钥
}

//json 文件读取的节点结构
type Node struct {
	NodeID     string // 节点ID
	NodePK     string //[]byte // 节点公钥信息
	NodeType   string // 节点类型，交易节点、共识节点等
	NodeStatus string // 节点状态
}

/*
 * 创建许可节点管理对象
 * 入参nodeprk，节点私钥
 */
func NewPermissionedNodeService(nodeprk *sm2.PrivateKey) (*PermissionedNodeService, error) {
	PNodes := make(map[string]PNode)
	// pnodes := statedb.GetPNodes(sc.SystemID, nil)
	nodes := make([]Node, 20)
	f, err := os.Open("author.json")
	d := json.NewDecoder(f)
	if err != nil {
		return nil, err
	}
	d.Decode(&nodes)
	for _, node := range nodes {
		if node.NodeStatus == "N_ABLED" {
			PNodes[node.NodeID] = PNode{
				ID:   node.NodeID,
				Type: node.NodeType,
				PK:   node.NodePK}
		}
	}
	return &PermissionedNodeService{NodePrk: nodeprk, pNodes: PNodes}, nil
}

// GetPermissionedNodes 获取所有许可节点的信息
// 返回map型数据，key为节点enode, 值为节点公钥
func (self *PermissionedNodeService) GetPermissionedNodes() map[string]string {
	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	// nodes := self.stateCache.GetPNodes(sc.SystemID, nil) //节点id，节点许可信息
	pnodes := make(map[string]string)
	for nid, ele := range self.pNodes {
		pnodes[nid] = ele.PK
	}
	return pnodes
}

// GetPermissionedConNodes 获取许可的共识节点
// 返回map型数据，key为节点enode, 值为节点公钥
func (self *PermissionedNodeService) GetPermissionedConNodes() sync.Map {
	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	var pcnodes sync.Map //(map[string]string) //许可的共识节点信息，key为节点ID，value为节点公钥地址
	//stateNodes := self.stateCache.GetPNodes(sc.SystemID, nil) //节点id，节点许可信息
	for nid, ele := range self.pNodes {
		if ele.Type == "ConsensusNode" {
			pcnodes.Store(nid, ele.PK)
		}
	}
	return pcnodes
}

//判断是否是许可的共识节点
func (self *PermissionedNodeService) JudgeConPermissioned(nid string) bool {
	// var pcnodes sync.Map //(map[string]string) //许可的共识节点信息，key为节点ID，value为节点公钥地址
	//node := self.stateCache.GetPNodeByNID(sc.SystemID, nil, nid) //节点id，节点许可信息

	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	if node, ok := self.pNodes[nid]; ok {
		if node.Type == "ConsensusNode" {
			return true
		}
	}
	return false
}

//判断节点是否被许可
func (self *PermissionedNodeService) JudgePermissioned(nid string) bool {
	// var pcnodes sync.Map //(map[string]string) //许可的共识节点信息，key为节点ID，value为节点公钥地址
	//node := self.stateCache.GetPNodeByNID(sc.SystemID, nil, nid) //节点id，节点许可信息

	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	if _, ok := self.pNodes[nid]; ok {
		return true
	}
	return false
}

// //判断是否是许可的交易节点
// func (self *PermissionedNodeService) JudgeTxPermissioned(nid string) bool {
// 	self.pnodesMux.RLock()
// 	defer self.pnodesMux.RUnlock()
// 	if node, ok := self.pNodes[nid]; ok {
// 		if node.Type == sc.TransactionNode {
// 			return true
// 		}
// 	}
// 	return false
// }

// // GetPermssionedTxNodes 获取许可的交易节点
// // 返回map型数据，key为节点enode, 值为节点公钥
// func (self *PermissionedNodeService) GetPermssionedTxNodes() map[string]string {
// 	//stateNodes := self.stateCache.GetPNodes(sc.SystemID, nil) //节点id，节点许可信息
// 	self.pnodesMux.RLock()
// 	defer self.pnodesMux.RUnlock()
// 	var pcnodes (map[string]string) //许可的交易节点信息，key为节点ID，value为节点公钥地址
// 	for nid, ele := range self.pNodes {
// 		if ele.Type == sc.TransactionNode {
// 			pcnodes[nid] = ele.PK
// 		}
// 	}
// 	return pcnodes
// }

// GetPKByPID 根据许可节点的ID获取节点的公钥
func (self *PermissionedNodeService) GetPKByPID(nid string) (string, error) {

	//FIXME 取前16位，因为peer中的id只取了前16位，有调用方的入参是通过peer.id传入
	// pid好像只有16位，这样是不是没办法获取到
	//node := self.stateCache.GetPNodeByNID(sc.SystemID, nil, nid) //节点id，节点许可信息
	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	if node, ok := self.pNodes[nid]; ok {
		return node.PK, nil
	} else {
		return "", errors.New("peer is not permissioned:" + nid[:len(nid)/16])
	}
}

func (self *PermissionedNodeService) GetBase64PKByPID(nid string) []byte {

	//FIXME 取前16位，因为peer中的id只取了前16位，有调用方的入参是通过peer.id传入
	// pid好像只有16位，这样是不是没办法获取到
	//node := self.stateCache.GetPNodeByNID(sc.SystemID, nil, nid) //节点id，节点许可信息
	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	if node, ok := self.pNodes[nid]; ok {
		decodeBytes, err := base64.StdEncoding.DecodeString(node.PK)
		if err != nil {
			glog.V(logger.Error).Infoln("decode nodepk Err")
			return nil
		}
		return decodeBytes
	} else {
		glog.V(logger.Error).Infoln("get nodepk Err")
		return nil
	}
}

//base64编码的
func (self *PermissionedNodeService) GetAllNodesPK64() map[string][]byte {

	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	//nodes := self.stateCache.GetPNodes(sc.SystemID, nil) //节点id，节点许可信息
	nodeandpk := make(map[string][]byte)
	for nid, node := range self.pNodes {
		decodeBytes, err := base64.StdEncoding.DecodeString(node.PK)
		if err != nil {
			glog.V(logger.Error).Infoln("decode nodepk Err")
			return nil
		}
		nodeandpk[nid] = decodeBytes
	}
	return nodeandpk
}

//base64编码的
func (self *PermissionedNodeService) GetConNodesPK64() map[string][]byte {
	self.pnodesMux.RLock()
	defer self.pnodesMux.RUnlock()
	//nodes := self.stateCache.GetPNodes(sc.SystemID, nil) //节点id，节点许可信息
	nodeandpk := make(map[string][]byte)
	for nid, node := range self.pNodes {
		if node.Type == "ConsensusNode" {
			decodeBytes, err := base64.StdEncoding.DecodeString(node.PK)
			if err != nil {
				glog.V(logger.Error).Infoln("decode nodepk Err")
				return nil
			}
			nodeandpk[nid] = decodeBytes
		}
	}
	return nodeandpk
}

////base64编码的 指定区块状态下的公钥
//func (self *PermissionedNodeService) GetAllNodesPK64ByState(root common.Hash) map[string][]byte {
//
//	stateAt, err := self.stateCache.New(root)
//	if err != nil {
//		glog.V(logger.Info).Infoln("new state by root Err", err)
//		return nil
//	}
//	nodes := stateAt.GetPNodes(sc.SystemID, nil) //节点id，节点许可信息
//	nodeandpk := make(map[string][]byte)
//	for nid, node := range nodes {
//		decodeBytes, err := base64.StdEncoding.DecodeString(string(node.NodePK))
//		if err != nil {
//			glog.V(logger.Error).Infoln("decode nodepk Err")
//			return nil
//		}
//		nodeandpk[nid] = decodeBytes
//	}
//	return nodeandpk
//}
//
////base64编码的 指定区块状态下的公钥
//func (self *PermissionedNodeService) GetConNodesPK64ByState(root common.Hash) map[string][]byte {
//
//	stateAt, err := self.stateCache.New(root)
//	if err != nil {
//		glog.V(logger.Info).Infoln("new state by root Err", err)
//		return nil
//	}
//	nodes := stateAt.GetPNodes(sc.SystemID, nil) //节点id，节点许可信息
//	nodeandpk := make(map[string][]byte)
//	for nid, node := range nodes {
//		if node.NodeType == sc.ConsensusNode{
//			decodeBytes, err := base64.StdEncoding.DecodeString(string(node.NodePK))
//			if err != nil {
//				glog.V(logger.Error).Infoln("decode nodepk Err")
//				return nil
//			}
//			nodeandpk[nid] = decodeBytes
//		}
//	}
//	return nodeandpk
//}

//如果许可节点length不一致，重新加载许可节点信息，返回true
func (self *PermissionedNodeService) ReloadPNodes(blockHeight uint64) bool {

	// self.pnodesMux.Lock()
	// defer self.pnodesMux.Unlock()
	// //避免收到重复的chainheadevent,重复reload
	// if blockHeight != 0 && blockHeight <= self.blockHeight {
	// 	return false
	// }
	// self.blockHeight = blockHeight
	// pnodes := self.stateCache.GetPNodes(sc.SystemID, nil)
	// if len(pnodes) != len(self.pNodes) { //FIXME 这种判断方式不完全有效，在同时增减或只是disabled情况下无效
	// 	PNodes := make(map[string]PNode)
	// 	for id, node := range pnodes {
	// 		if node.NodeStatus == sc.N_ABLED {
	// 			PNodes[id] = PNode{
	// 				ID:   id,
	// 				Type: node.NodeType,
	// 				PK:   string(node.NodePK)}
	// 		}
	// 	}
	// 	self.pNodes = PNodes
	// 	return true
	// }
	return false
}
