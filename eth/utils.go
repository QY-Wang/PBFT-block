package eth

import (
	"bufio"
	"encoding/json"
	crypto "github.com/DWBC-ConPeer/crypto-gm"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/p2p/discover"
	uuid "github.com/satori/go.uuid"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"time"
)

type JsonPrivateKey struct {
	PublicKey []byte
	D         *big.Int `json:"D"`
}

type Config struct {
	Myport   string
	Peersnum int
	Peers    []string
}

type LogMsg struct {
	//发送的消息
	Sender  string
	Time    time.Time
	Content string
	Other   string
}
type LogMsg2 struct {
	//发送的消息
	From    string
	Time    time.Time
	Content string
	Other   string
}

func Returnadd(id string) string {
	if id == "c1ac603bb0cdb6d2948a9e7cf40dcf86da2cd95644ec34a0b09ff13024859963839ec837969a51ffe3aa51a1e26a2631c61eea788e6a35a01ca73696bb852319" {
		return "127.0.0.1" + Con.Peers[0]
	}
	if id == "b6736ae4d1734f6d29592f0db4480fd5dd6d98715e0981dbdd2489c14acb57958cda259bb064662f1237cbf5271cf3ee6dc9ac5afb7b4dad5104e244ce996f08" {
		return "127.0.0.1" + Con.Peers[1]
	}
	if id == "3ac2a528373f175454830cb57930876d861ea6d9ce24a2aa507b7e6e077fd19771018b95e7bafc8f6a9a66eefb7d6ef9f8a6afa73f84e399741335e541cb8be2" {
		return "127.0.0.1" + Con.Peers[2]
	}
	if id == "48de9fe6797554d14ab123a26d611326635c49178bf86c86a50cfd807be6735a7c71f78bf6668e563be8315ab1c599ba6bbb20d4cba4a865742773ef46c0b042" {
		return "127.0.0.1" + Con.Peers[3]
	}
	return ""
}
func Writelog(add string, data interface{}) {
	newdata, _ := json.MarshalIndent(data, "", "    ")
	newdata = []byte(string(newdata) + "\n")
	file, _ := os.OpenFile(add, os.O_RDWR|os.O_APPEND, 0667)
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.Write(newdata)
	writer.Flush()
}

var datadirStaticNodes string = "static-nodes.json"

var datadirTrustedNodes string = "trusted-nodes.json"

func StaticNodes() []*discover.Node {
	return parsePersistentNodes(datadirStaticNodes)
}

// TrusterNodes returns a list of node enode URLs configured as trusted nodes.
func TrusterNodes() []*discover.Node {
	return parsePersistentNodes(datadirTrustedNodes)
}
func parsePersistentNodes(file string) []*discover.Node {
	// Short circuit if no node config is present
	if _, err := os.Stat(file); err != nil {
		return nil
	}
	// Load the nodes from the config file
	blob, err := ioutil.ReadFile(file)
	if err != nil {
		glog.V(logger.Error).Infof("Failed to access nodes: %v", err)
		return nil
	}
	nodelist := []string{}
	if err := json.Unmarshal(blob, &nodelist); err != nil {
		glog.V(logger.Error).Infof("Failed to load nodes: %v", err)
		return nil
	}
	// Interpret the list as a discovery node array
	var nodes []*discover.Node
	for _, url := range nodelist {
		if url == "" {
			continue
		}
		node, err := discover.ParseNode(url)
		if err != nil {
			glog.V(logger.Error).Infof("Node URL %s: %v\n", url, err)
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func NodeKey() *sm2.PrivateKey { //私钥转成字符串再存储
	//  私钥文件存在
	pk, _ := crypto.LoadECDSA("privatekey.txt")
	if pk != nil {
		return pk
	}
	// 如果私钥文件不存在
	_, err3 := os.OpenFile("privatekey.txt", os.O_CREATE|os.O_RDWR, 0777)
	if err3 != nil {
		log.Fatal(err3.Error())
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		glog.Fatalf("Failed to generate node key: %v", err)
	}
	crypto.SaveECDSA("privatekey.txt", key)
	return key
}
func FormatOutput(Sender string, Reciver string, mess string) { //兼容前台的标准型日志输出方法
	Messid := uuid.NewV4().String()
	f, err := os.OpenFile("flog.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, os.ModePerm)
	if err != nil {
		return
	}
	defer func() {
		f.Close()
	}()
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println(Messid + "_" + Sender + "_" + Reciver + "_" + mess) //第一位发送者ip端口，第二位接受者ip端口，第三位传输信息的具体内容
	//fmt.Println("logout: %v ", logtext)
}
