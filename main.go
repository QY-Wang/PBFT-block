package main

import (
	"fmt"
	"github.com/DWBC-ConPeer/eth"
	"github.com/DWBC-ConPeer/p2p/discover"
	"sync"
	"time"
)

var wg sync.WaitGroup

func main() {
	wg.Add(1)
	eth.ReadandWrite()
	eth.InitP2PNode()
	err := eth.StartP2PNode()
	if err != nil {
		panic(err.Error())
	}
	time.AfterFunc(time.Hour, func() {
		wg.Done()
	})
	//time.Sleep(15 * time.Second)
	eth.Initt()
	fmt.Println(discover.PubkeyID(&eth.NodeKey().PublicKey).String())
	//fmt.Println(eth.Node1.N)
	//eth.Node1.StartConsensus()
	//time.Sleep(60 * time.Second)
	if eth.Node1.GetSelfNodeID() == "127.0.0.1"+eth.Con.Peers[0] {
		t := time.NewTicker(time.Second * 2)
		for v := range t.C {
			if len(eth.Pm.Peers.GetPeers()) == eth.Con.Peersnum-1 {
				time.Sleep(15 * time.Second)
				eth.Node1.StartConsensus()
				break
			}
			fmt.Println(len(eth.Pm.Peers.GetPeers()), v)
		}
	} else {
		t := time.NewTicker(time.Second * 1)
		for v := range t.C {
			fmt.Println("我不是主节点,目前peer数量：", len(eth.Pm.Peers.GetPeers()), v)
		}
	}
	//t := time.NewTicker(time.Second * 2)
	//for v := range t.C {
	//	if len(eth.Pm.Peers.GetPeers()) == 3 {
	//		eth.Node1.StartConsensus()
	//		break
	//	}
	//	fmt.Println(len(eth.Pm.Peers.GetPeers()), v)
	//}

	//eth.CycleSendMsg()
	//tool.Node1.StartConsensus(1)
	wg.Wait()
}
