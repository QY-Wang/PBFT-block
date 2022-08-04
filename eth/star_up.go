package eth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	crypto "github.com/DWBC-ConPeer/crypto-gm"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/event"
	"github.com/DWBC-ConPeer/p2p"
	"github.com/DWBC-ConPeer/p2p/discover"
	"github.com/DWBC-ConPeer/pns"
)

var privatekey *sm2.PrivateKey
var Pm *ProtocolManager
var EventMux *event.TypeMux
var pnService *pns.PermissionedNodeService
var Node1 = new(worker)
var Con Config

func ReadandWrite() {

	var config Config
	f, err := os.Open("config.json")
	d := json.NewDecoder(f)
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()
	d.Decode(&config)
	fmt.Println(config.Myport)
	//fmt.Println(len(config.Peers))
	fmt.Println(config.Peers)
	fmt.Println(config.Peersnum)
	//获得这个节点是数组里边的第几个
	var serialnumber int
	fmt.Println(config.Myport)
	for i := 0; i < config.Peersnum; i++ {
		fmt.Println(config.Peers[i])
		if config.Myport == config.Peers[i] {
			serialnumber = i + 1
			fmt.Println(config.Peers[i])
		} else {

		}
	}
	fmt.Printf("serialnumber")
	fmt.Println(serialnumber)
	serialnumberstring := strconv.Itoa(serialnumber)
	//来修改privatekey.txt
	var privatekeyname string
	privatekeyname = "privatekey" + serialnumberstring + ".txt"
	fmt.Println(privatekeyname)
	data, err := ioutil.ReadFile(privatekeyname)
	if err != nil {
		fmt.Printf("文件打开失败=%v\n", err)
		return
	}
	err = ioutil.WriteFile("privatekey.txt", data, 0666)
	if err != nil {
		fmt.Printf("文件打开失败=%v\n", err)
	}
	//先修改P2PConfig.json
	var conf PartsP2PConfig
	e, _ := ioutil.ReadFile("P2PConfig.json")
	_ = json.Unmarshal(e, &conf)
	fmt.Println(conf)
	//oldport := conf.ListenAddr
	conf.ListenAddr = config.Myport
	conf.Name = config.Myport
	result, _ := json.MarshalIndent(conf, "", " ")
	_ = ioutil.WriteFile("P2PConfig.json", result, 0644)

	//读取anthor.json

	// pnodes := statedb.GetPNodes(sc.SystemID, nil)
	nodes := make([]pns.Node, 20)
	file1, err := os.Open("author.json")
	file1d := json.NewDecoder(file1)
	if err != nil {
		fmt.Print("1111")
	}
	file1d.Decode(&nodes)
	//从nodes中删除
	newnodes := append(nodes[:serialnumber-1], nodes[serialnumber:]...)
	staticports := append(config.Peers[:serialnumber-1], config.Peers[serialnumber:]...)
	nodelist := make([]string, config.Peersnum-1)
	for i := 0; i < len(newnodes); i++ {
		temresult := "enode://" + newnodes[i].NodeID + "@127.0.0.1" + staticports[i]
		fmt.Printf(strconv.Itoa(i))
		fmt.Println(temresult)
		nodelist[i] = temresult
	}
	//修改静态节点文件

	nodelistresult, _ := json.Marshal(nodelist)
	_ = ioutil.WriteFile("static-nodes.json", nodelistresult, 0644)
	fmt.Println(nodelist)

}
func Initt() {
	file, _ := os.Open("config.json")
	// 关闭文件
	defer file.Close()
	// NewDecoder创建一个从file读取并解码json对象的*Decoder，解码器有自己的缓冲，并可能超前读取部分json数据。
	decoder := json.NewDecoder(file)
	//Decode从输入流读取下一个json编码值并保存在v指向的值里
	err := decoder.Decode(&Con)
	if err != nil {
		panic(err)
	}
	Node1.nodeID = "127.0.0.1" + Con.Myport
	Node1.view = 1
	//leader统一选择第一个
	Node1.leader = "127.0.0.1" + Con.Peers[0]
	Node1.LastReqID = 1
	chainn := new(BlockChain)
	chainn.CurrentBlock = 1
	Node1.chain = chainn
	Node1.ready = true

	Node1.config = new(ChainConfig)
	Node1.mux = EventMux
	//Node1.chain = new(BlockChain)
	Node1.current = new(Work)
	Node1.certStore = map[msgID]*msgCert{}
	Node1.Stack = new(Node)
	Node1.PNService = new(PermissionedNodeService)
	Node1.possibleValidBlock = map[uint64]map[string]*Block{}
	Node1.ccvm = new(DWCCVM)
	Node1.blockbeats = map[string]time.Time{}
	Node1.agents = map[Agent]struct{}{}
	Node1.dbs = map[string]Database{}
	Node1.recv = make(chan *Result)
	Node1.crcertstore = map[uint64]map[string]ConsensuReponse{}
	Node1.newLeaderSetStore = map[string]NewLeaderSet{}
	Node1.N = Con.Peersnum
	Node1.PbftMux = EventMux
}

type PartsP2PConfig struct {
	MaxPeers        int
	MaxPendingPeers int
	Discovery       bool
	Name            string
	NodeDatabase    string
	NoDial          bool
	ListenAddr      string
}

type TestMsg struct {
	Msg    string
	Sender string
	Time   string
}

func InitP2PNode() {
	EventMux = new(event.TypeMux)
	NetworkId := 20210704
	var err error
	privatekey = NodeKey()
	// fmt.Printf("privatekey: %v\n", *privatekey)
	var err2 error
	pnService, err2 = pns.NewPermissionedNodeService(privatekey)
	if err2 != nil {
		log.Fatalln(err2.Error())
	}
	Pm, err = NewProtocolManager(NetworkId, EventMux, pnService)
	if err != nil {
		log.Fatalln(err.Error())
	}
	nodeID := discover.PubkeyID(&privatekey.PublicKey).String()
	fmt.Printf("nodeID: %v\n", nodeID)
	base64PubilcKey := base64.StdEncoding.EncodeToString(crypto.FromECDSAPub(&privatekey.PublicKey))
	fmt.Printf("base64PubilcKey: %v\n", base64PubilcKey)
	fmt.Printf("base64.StdEncoding.EncodeToString([]byte(base64PubilcKey)): %v\n", base64.StdEncoding.EncodeToString([]byte(base64PubilcKey)))

}

func StartP2PNode() error {
	f, err := os.Open("P2PConfig.json")
	if err != nil {
		panic(err.Error())
	}
	defer f.Close()
	d := json.NewDecoder(f)
	var conf PartsP2PConfig
	d.Decode(&conf)
	p := Pm.SubProtocols
	var config = p2p.Config{
		PrivateKey:      privatekey,
		Name:            conf.Name,
		Discovery:       false,
		BootstrapNodes:  nil,
		StaticNodes:     StaticNodes(),
		TrustedNodes:    TrusterNodes(),
		NodeDatabase:    conf.NodeDatabase,
		ListenAddr:      conf.ListenAddr,
		NAT:             nil,
		Dialer:          nil,
		NoDial:          conf.NoDial,
		MaxPeers:        conf.MaxPeers,
		MaxPendingPeers: conf.MaxPendingPeers,
	}
	Pm.Start() //启动protocolmanager
	// fmt.Println("protocolmanager启动!")
	running := &p2p.Server{Config: config}
	running.Protocols = append(running.Protocols, p...)
	if err1 := running.Start(); err1 != nil {
		return err1
	}
	return nil
}

func CycleSendMsg() {
	//t := time.NewTicker(time.Second * 5)
	//for i := 0; i < 100; i++ {
	//var TestSendMsg = TestMsg{
	//	Msg:    "TestSendMsg",
	//	Sender: "Node2", // 将公钥转为base64
	//	Time:   v.Format("2006/01/02 15:04:05"),
	//}
	var pre = Prepare{
		Sender:         "测试节点",
		View:           1,
		Timestamp:      time.Now(),
		ReqID:          1,
		RequestDigest:  "test",
		TxsExeSeqs:     []byte("test"),
		RealTxsExeSeqs: nil,
	}
	var temp, _ = json.Marshal(pre)
	var A = ConsensusMsg{
		Msg:       string(temp),
		Timestamp: time.Now(),
		Type:      "prepare",
		Sender:    "node1",
	}
	var B = Checkpoint{
		Sender:      "node1",
		BlockHeight: 1,
		ReqID:       1,
		BlockHash:   "哈希",
	}
	EventMux.Post(A)
	EventMux.Post(B)
	//fmt.Println(v)
	//EventMux.Post(TestSendMsg)
	//}
}
