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

// Package p2p implements the Ethereum p2p network protocols.
package p2p

import (
	"errors"
	"fmt"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/crypto-gm/sm3"
	"net"
	"sync"
	"time"

	crypto "github.com/DWBC-ConPeer/crypto-gm"

	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/p2p/discover"
	"github.com/DWBC-ConPeer/p2p/nat"
)

const (
	defaultDialTimeout      = 15 * time.Second
	refreshPeersInterval    = 30 * time.Second
	staticPeerCheckInterval = 15 * time.Second

	// Maximum number of concurrently handshaking inbound connections. 同时进行握手的最大入站连接数
	maxAcceptConns = 50

	// Maximum number of concurrently dialing outbound connections.  最多同时拨出的外线连接数
	maxActiveDialTasks = 16

	// Maximum time allowed for reading a complete message.
	// This is effectively the amount of time a connection can be idle.
	// 读取一条完整信息所允许的最长时间。
	// 这实际上是一个连接可以空闲的时间。
	frameReadTimeout = 30 * time.Second

	// Maximum amount of time allowed for writing a complete message.
	//撰写一份完整信息的最大允许时间
	frameWriteTimeout = 20 * time.Second
)

type PublicServer struct {
	Name string
}

var errServerStopped = errors.New("server stopped")

var srvjslog = logger.NewJsonLogger()

// Config holds Server options. 配置持有服务器选项
type Config struct {
	// This field must be set to a valid secp256k1 private key.  这个字段必须设置为有效的secp256k1私钥
	PrivateKey *sm2.PrivateKey

	// MaxPeers is the maximum number of peers that can be
	// connected. It must be greater than zero.  MaxPeers是可以连接的最大对等体数量。连接。它必须大于零
	MaxPeers int

	// MaxPendingPeers is the maximum number of peers that can be pending in the
	// handshake phase, counted separately for inbound and outbound connections.
	// Zero defaults to preset values.
	//MaxPendingPeers是指在握手阶段可以挂起的最大对等体数量，对入站和出站连接分别进行计算。
	//零默认为预设值
	MaxPendingPeers int

	// Discovery specifies whether the peer discovery mechanism should be started
	// or not. Disabling is usually useful for protocol debugging (manual topology).
	//发现指定是否应启动对等体发现机制 或不启动。禁用通常对协议调试有用（手动拓扑
	Discovery bool

	// Name sets the node name of this server.
	// Use common.MakeName to create a name that follows existing conventions.
	// Name设置该服务器的节点名称。
	// 使用common.MakeName来创建一个遵循现有惯例的名称。
	Name string

	// Bootstrap nodes are used to establish connectivity
	// with the rest of the network. Bootstrap 用于建立与其他网络的连接 与网络的其他部分建立连接。
	BootstrapNodes []*discover.Node

	// Static nodes are used as pre-configured connections which are always
	// maintained and re-connected on disconnects.
	//静态节点被用作预先配置的连接，这些连接始终被 保持并在断开连接时重新连接
	StaticNodes []*discover.Node

	// Trusted nodes are used as pre-configured connections which are always
	// allowed to connect, even above the peer limit. 受信任的节点被用作预先配置的连接，它们总是被允许连接，甚至超过对等体的限制
	TrustedNodes []*discover.Node

	// NodeDatabase is the path to the database containing the previously seen
	// live nodes in the network. NodeDatabase是数据库的路径，其中包含之前看到的的实时节点
	NodeDatabase string

	// Protocols should contain the protocols supported
	// by the server. Matching protocols are launched for
	// each peer.协议应包含服务器支持的协议。 为每个对等点启动匹配协议
	Protocols []Protocol

	// If ListenAddr is set to a non-nil address, the server
	// will listen for incoming connections.
	//
	// If the port is zero, the operating system will pick a port. The
	// ListenAddr field will be updated with the actual address when
	// the server is started.
	// 如果 ListenAddr 设置为非零地址，则服务器将侦听传入连接。
	// 如果端口为零，操作系统将选择一个端口。 当服务器启动时，ListenAddr 字段将更新为实际地址。
	ListenAddr string

	// If set to a non-nil value, the given NAT port mapper
	// is used to make the listening port available to the
	// Internet. 如果设置为非零值，则使用给定的 NAT 端口映射器使侦听端口可用于 Internet
	NAT nat.Interface

	// If Dialer is set to a non-nil value, the given Dialer
	// is used to dial outbound peer connections. 如果 Dialer 设置为非零值，则给定的 Dialer 用于拨打出站对等连接。
	Dialer *net.Dialer

	// If NoDial is true, the server will not dial any peers. 如果NoDial为真，服务器将不拨打任何对等体。
	NoDial bool
}

// Server manages all peer connections.  服务器管理所有对等连接
type Server struct {
	// Config fields may not be modified while the server is running.  当服务器运行时，配置字段不能被修改
	Config

	// Hooks for testing. These are useful because we can inhibit
	// the whole protocol stack. 用于测试的钩子。这些是很有用的，因为我们可以抑制整个协议栈
	NewTransport func(net.Conn) transport
	NewPeerHook  func(*Peer)

	lock    sync.Mutex // protects running
	running bool

	ntab         discoverTable
	listener     net.Listener    // 侦听器是面向流协议的通用网络侦听器。 多个 goroutine 可以同时调用 Listener 上的方法。
	ourHandshake *protoHandshake // protoHandshake是协议握手的RLP结构
	lastLookup   time.Time

	// These are for Peers, PeerCount (and nothing else).  这些是针对 Peers、PeerCount 的（仅此而已）。
	peerOp     chan peerOpFunc
	peerOpDone chan struct{}

	quit          chan struct{}
	addstatic     chan *discover.Node /**ConNode*/
	posthandshake chan *conn
	addpeer       chan *conn
	delpeer       chan *Peer
	loopWG        sync.WaitGroup // loop, listenLoop  循环，听循环
	NodeType      uint64         // 节点类型为 1 ，固化至代码中
}

type ConNode struct {
	CNode     *discover.Node
	CNodeType uint64
}

type peerOpFunc func(map[discover.NodeID]*Peer)

type connFlag int

const (
	dynDialedConn connFlag = 1 << iota
	staticDialedConn
	inboundConn
	trustedConn
)

// conn wraps a network connection with information gathered
// during the two handshakes.
// conn用两次握手期间收集到的信息来包装一个网络连接
type conn struct {
	fd net.Conn
	transport
	flags    connFlag
	cont     chan error      // The run loop uses cont to signal errors to setupConn. 运行循环使用cont来向setupConn发出错误信号。
	id       discover.NodeID // valid after the encryption handshake  加密握手后有效  NodeID是每个节点的唯一标识符。节点标识符是一个帅气的椭圆曲线公钥。
	caps     []Cap           // valid after the protocol handshake 在协议握手后有效
	name     string          // valid after the protocol handshake 在协议握手后有效
	NodeType uint64          // 节点类型, 共识节点为1
}

/*
*** 新节点的 NodeType 默认值为 0 . 表示暂时链接的状态 作为中转状态
*** 0 类型的节点可以与其他节点通信，但是通信消息受限
 */

type transport interface {
	// The two handshakes.  两次握手
	doEncHandshake(prv *sm2.PrivateKey, dialDest *discover.Node) (discover.NodeID, error)
	doProtoHandshake(our *protoHandshake) (*protoHandshake, error)
	// The MsgReadWriter can only be used after the encryption
	// handshake has completed. The code uses conn.id to track this
	// by setting it to a non-nil value after the encryption handshake.
	// MsgReadWriter只能在加密后使用。
	// 握手完成后才能使用。代码使用conn.id来跟踪，在加密握手后将其设置为一个非零值
	MsgReadWriter
	// transports must provide Close because we use MsgPipe in some of
	// the tests. Closing the actual network connection doesn't do
	// anything in those tests because NsgPipe doesn't use it.
	// transports必须提供Close，因为我们在一些测试中使用MsgPipe。关闭实际的网络连接在这些测试中没有任何作用，因为NsgPipe不使用它。
	close(err error)
}

func (c *conn) String() string {
	s := c.flags.String() + " conn"
	if (c.id != discover.NodeID{}) {
		s += fmt.Sprintf(" %x", c.id[:8])
	}
	s += " " + c.fd.RemoteAddr().String()
	return s
}

func (f connFlag) String() string {
	s := ""
	if f&trustedConn != 0 {
		s += " trusted"
	}
	if f&dynDialedConn != 0 {
		s += " dyn dial"
	}
	if f&staticDialedConn != 0 {
		s += " static dial"
	}
	if f&inboundConn != 0 {
		s += " inbound"
	}
	if s != "" {
		s = s[1:]
	}
	return s
}

func (c *conn) is(f connFlag) bool {
	return c.flags&f != 0
}

// Peers returns all connected peers.
func (srv *Server) Peers() []*Peer {
	var ps []*Peer
	select {
	// Note: We'd love to put this function into a variable but
	// that seems to cause a weird compiler error in some
	// environments. //注意：我们很想把这个函数放到一个变量中，但是
	//但在某些环境下，这似乎会导致一个奇怪的编译器错误。
	case srv.peerOp <- func(peers map[discover.NodeID]*Peer) {
		for _, p := range peers {
			ps = append(ps, p)
		}
	}:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return ps
}

// PeerCount returns the number of connected peers.
func (srv *Server) PeerCount() int {
	var count int
	select {
	case srv.peerOp <- func(ps map[discover.NodeID]*Peer) { count = len(ps) }:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return count
}

// AddPeer connects to the given node and maintains the connection until the
// server is shut down. If the connection fails for any reason, the server will
// attempt to reconnect the peer.
//func (srv *Server) AddPeer(node *discover.Node) {
//	select {
//	case srv.addstatic <- node:
//	case <-srv.quit:
//	}
//}

/*
*  AddPeerByType 添加不同类型的节点
*  nodeType 1 表示 共识节点
*           2 表示 账本节点 , 不参与共识但存储账本
*           3 表示候选共识节点
*           4 表示种子节点bootstrap节点
			5 表示轻节点 ，暂不实现
* 为支持 addPeerByType 的实现,需要改变 addstatic 的结构，chan *ConNode
* type ConNode struct{
*    CNode *discover.Node
*	 CNodeType uint64
* }
* 同时, 相关 addstatic 的调用函数均需改变
* 这样做的好处是：所有联通节点仍维护在同一个队列中
* 由调用者根据分类去进行业务逻辑判断
*/

func (srv *Server) AddPeerByType(node *discover.Node, nodetype uint64) {
	//	newconNode := &ConNode{
	//		CNode:     node,
	//		CNodeType: nodetype,
	//	}
	select {
	case srv.addstatic <- node:
	case <-srv.quit:
	}
}

/*
* 设置本节点类型
*  nodeType 1 表示 共识节点
*           2 表示 账本节点 , 不参与共识但存储账本
*           3 表示候选共识节点
*           4 表示种子节点bootstrap节点
			5 表示轻节点 ，暂不实现
*/
func (srv *Server) SetSelfNodeType(nodetype uint64) bool {
	srv.NodeType = nodetype
	// TODO 使之变化，如共识节点的恒为1
	return true
}

func (srv *Server) GetSelfNodeType() uint64 {
	if srv.NodeType == 0 {
		return uint64(2)
	} else {
		return srv.NodeType
	}
}

// Self returns the local node's endpoint information.
func (srv *Server) Self() *discover.Node {
	srv.lock.Lock()
	defer srv.lock.Unlock()

	// If the server's not running, return an empty node
	if !srv.running {
		return &discover.Node{IP: net.ParseIP("0.0.0.0")}
	}
	// If the node is running but discovery is off, manually assemble the node infos  如果节点正在运行，但发现是关闭的，手动组装节点信息
	if srv.ntab == nil {
		// Inbound connections disabled, use zero address  禁止入站连接，使用零地址
		if srv.listener == nil {
			return &discover.Node{IP: net.ParseIP("0.0.0.0"), ID: discover.PubkeyID(&srv.PrivateKey.PublicKey)}
		}
		// Otherwise inject the listener address too
		addr := srv.listener.Addr().(*net.TCPAddr)
		return &discover.Node{
			ID:       discover.PubkeyID(&srv.PrivateKey.PublicKey),
			IP:       addr.IP,
			TCP:      uint16(addr.Port),
			NodeType: uint64(1), // 共识节点，故 nodetype 为 1
		}
	}
	// Otherwise return the live node infos
	return srv.ntab.Self()
}

// Stop terminates the server and all active peer connections.
// It blocks until all active connections have been closed.
func (srv *Server) Stop() {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if !srv.running {
		return
	}
	srv.running = false
	if srv.listener != nil {
		// this unblocks listener Accept
		srv.listener.Close()
	}
	close(srv.quit)
	srv.loopWG.Wait()
}

// Start starts running the server.
// Servers can not be re-used after stopping.
// Start开始运行服务器。
// 服务器在停止后不能被重新使用。
func (srv *Server) Start() (err error) {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if srv.running {
		return errors.New("server already running")
	}
	srv.running = true
	glog.V(logger.Info).Infoln("Starting Server")

	// static fields
	if srv.PrivateKey == nil {
		return fmt.Errorf("Server.PrivateKey must be set to a non-nil key")
	}
	if srv.NewTransport == nil {
		srv.NewTransport = newRLPX
	}
	if srv.Dialer == nil { //修改的部分
		srv.Dialer = &net.Dialer{Timeout: defaultDialTimeout}
		//var localaddr net.TCPAddr
		//localaddr.IP = net.ParseIP("127.0.0.1")
		//localaddr.Port, _ = strconv.Atoi(srv.Config.ListenAddr[1:])
		//srv.Dialer = &net.Dialer{Timeout: defaultDialTimeout, LocalAddr: &localaddr}
	}
	srv.quit = make(chan struct{})
	srv.addpeer = make(chan *conn)
	srv.delpeer = make(chan *Peer)
	srv.posthandshake = make(chan *conn)
	srv.addstatic = make(chan *discover.Node)
	srv.peerOp = make(chan peerOpFunc)
	srv.peerOpDone = make(chan struct{})

	/*
	 *
	 */
	// node table
	if srv.Discovery {
		ntab, err := discover.ListenUDP(srv.PrivateKey, srv.ListenAddr, srv.NAT, srv.NodeDatabase) //ListenUDP返回一个新的表，监听laddr上的UDP数据包
		if err != nil {
			return err
		}
		if err := ntab.SetFallbackNodes(srv.BootstrapNodes); err != nil { // SetFallbackNodes设置初始联系点。这些节点用于连接到网络，在表是空的，并且数据库中没有已知的节点情况下。
			return err
		}
		srv.ntab = ntab
	}

	dynPeers := (srv.MaxPeers + 1) / 2
	if !srv.Discovery {
		dynPeers = 0
	}
	dialer := newDialState(srv.StaticNodes, srv.ntab, dynPeers)

	// handshake
	srv.ourHandshake = &protoHandshake{Version: baseProtocolVersion, Name: srv.Name, ID: discover.PubkeyID(&srv.PrivateKey.PublicKey), NodeType: uint64(1)}
	// fmt.Printf("srv.Protocols: %v\n", srv.Protocols)
	for _, p := range srv.Protocols {
		srv.ourHandshake.Caps = append(srv.ourHandshake.Caps, p.cap())
	}
	// listen/dial
	if srv.ListenAddr != "" {
		if err := srv.startListening(); err != nil {
			return err
		}
		// fmt.Println("startListening") 测试用
	}
	if srv.NoDial && srv.ListenAddr == "" {
		glog.V(logger.Warn).Infoln("I will be kind-of useless, neither dialing nor listening.")
	}

	srv.loopWG.Add(1)
	go srv.run(dialer)
	srv.running = true
	return nil
}

func (srv *Server) startListening() error {
	// Launch the TCP listener.
	listener, err := net.Listen("tcp", srv.ListenAddr)
	if err != nil {
		return err
	}
	laddr := listener.Addr().(*net.TCPAddr)
	srv.ListenAddr = laddr.String()
	srv.listener = listener
	srv.loopWG.Add(1)
	go srv.listenLoop()
	// Map the TCP listening port if NAT is configured. 如果配置了 NAT，则映射 TCP 侦听端口
	if !laddr.IP.IsLoopback() && srv.NAT != nil {
		srv.loopWG.Add(1)
		go func() {
			nat.Map(srv.NAT, srv.quit, "tcp", laddr.Port, laddr.Port, "ethereum p2p")
			srv.loopWG.Done()
		}()
	}
	return nil
}

type dialer interface {
	newTasks(running int, peers map[discover.NodeID]*Peer, now time.Time) []task
	taskDone(task, time.Time)
	addStatic(*discover.Node)
}

func (srv *Server) run(dialstate dialer) {
	defer srv.loopWG.Done()
	var (
		peers        = make(map[discover.NodeID]*Peer)
		trusted      = make(map[discover.NodeID]bool, len(srv.TrustedNodes))
		taskdone     = make(chan task, maxActiveDialTasks)
		runningTasks []task
		queuedTasks  []task // tasks that can't run yet
	)
	// Put trusted nodes into a map to speed up checks.
	// Trusted peers are loaded on startup and cannot be
	// modified while the server is running.
	// 把受信任的节点放到一个地图中，以加快检查速度。
	// 受信任的对等体在启动时被加载，在服务器运行时不能被修改
	for _, n := range srv.TrustedNodes {
		trusted[n.ID] = true
	}

	// removes t from runningTasks 从runningTasks中删除t
	delTask := func(t task) {
		for i := range runningTasks {
			if runningTasks[i] == t {
				runningTasks = append(runningTasks[:i], runningTasks[i+1:]...)
				break
			}
		}
	}
	// starts until max number of active tasks is satisfied  开始直到满足最大活动任务数
	startTasks := func(ts []task) (rest []task) {
		i := 0
		for ; len(runningTasks) < maxActiveDialTasks && i < len(ts); i++ {
			t := ts[i]
			glog.V(logger.Detail).Infoln("new task:", t)
			go func() { t.Do(srv); taskdone <- t }()
			runningTasks = append(runningTasks, t)
		}
		return ts[i:]
	}
	scheduleTasks := func() {
		// Start from queue first.
		queuedTasks = append(queuedTasks[:0], startTasks(queuedTasks)...)
		// Query dialer for new tasks and start as many as possible now. 查询拨号器以获取新任务并立即启动尽可能多的任务
		if len(runningTasks) < maxActiveDialTasks {
			nt := dialstate.newTasks(len(runningTasks)+len(queuedTasks), peers, time.Now())
			queuedTasks = append(queuedTasks, startTasks(nt)...)
		}
	}

running:
	for {
		scheduleTasks()

		select {
		case <-srv.quit:
			// The server was stopped. Run the cleanup logic.
			glog.V(logger.Detail).Infoln("<-quit: spinning down")
			break running
		case n := <-srv.addstatic:
			// This channel is used by AddPeer to add to the
			// ephemeral static peer list. Add it to the dialer,
			// it will keep the node connected.
			glog.V(logger.Detail).Infoln("<-addstatic:", n)

			dialstate.addStatic(n)
		case op := <-srv.peerOp:
			// This channel is used by Peers and PeerCount.
			op(peers)
			srv.peerOpDone <- struct{}{}
		case t := <-taskdone:
			// A task got done. Tell dialstate about it so it
			// can update its state and remove it from the active
			// tasks list.  一个任务完成了。 告诉 dialstate 以便它可以更新其状态并将其从 activetasks 列表中删除
			glog.V(logger.Detail).Infoln("<-taskdone:", t)
			dialstate.taskDone(t, time.Now())
			delTask(t)
		case c := <-srv.posthandshake:
			// A connection has passed the encryption handshake so  连接已通过加密握手，因此远程身份是已知的（但尚未验证）
			// the remote identity is known (but hasn't been verified yet).
			if trusted[c.id] {
				// Ensure that the trusted flag is set before checking against MaxPeers. 确保在检查 MaxPeers 之前设置了受信任的标志
				c.flags |= trustedConn
			}
			glog.V(logger.Detail).Infoln("<-posthandshake:", c)
			// TODO: track in-progress inbound node IDs (pre-Peer) to avoid dialing them.
			c.cont <- srv.encHandshakeChecks(peers, c)
		case c := <-srv.addpeer:
			// At this point the connection is past the protocol handshake.
			// Its capabilities are known and the remote identity is verified.
			// 此时连接已通过协议握手。
			// 它的功能是已知的，并且远程身份是经过验证的
			glog.V(logger.Detail).Infoln("<-addpeer:", c)
			err := srv.protoHandshakeChecks(peers, c)
			if err != nil {
				glog.V(logger.Detail).Infof("Not adding %v as peer: %v", c, err)
			} else {
				// The handshakes are done and it passed all checks. 握手完成并通过了所有检查
				p := newPeer(c, srv.Protocols)
				peers[c.id] = p
				go srv.runPeer(p)
			}
			// The dialer logic relies on the assumption that
			// dial tasks complete after the peer has been added or
			// discarded. Unblock the task last.
			c.cont <- err
		case p := <-srv.delpeer:
			// A peer disconnected.
			glog.V(logger.Detail).Infoln("<-delpeer:", p)
			delete(peers, p.ID())
		}
	}

	// Terminate discovery. If there is a running lookup it will terminate soon.
	if srv.ntab != nil {
		srv.ntab.Close()
	}
	// Disconnect all peers.
	for _, p := range peers {
		p.Disconnect(DiscQuitting)
	}
	// Wait for peers to shut down. Pending connections and tasks are
	// not handled here and will terminate soon-ish because srv.quit
	// is closed. 等待对等体关闭。 未处理的连接和任务在这里不处理，并且将很快终止，因为 srv.quit 已关闭
	glog.V(logger.Detail).Infof("ignoring %d pending tasks at spindown", len(runningTasks))
	for len(peers) > 0 {
		p := <-srv.delpeer
		glog.V(logger.Detail).Infoln("<-delpeer (spindown):", p)
		delete(peers, p.ID())
	}
}

func (srv *Server) protoHandshakeChecks(peers map[discover.NodeID]*Peer, c *conn) error {
	/*
	* 增加节点类型判断
	* 如果是4(即移动设备节点), do nothing
	 */

	if c.NodeType == uint64(0) || c.NodeType == uint64(4) {
		// TODO something should do
		glog.V(logger.Info).Infoln("Moving transaction")
		return nil
	}

	// Drop connections with no matching protocols. 丢弃没有匹配协议的连接
	if len(srv.Protocols) > 0 && countMatchingProtocols(srv.Protocols, c.caps) == 0 {
		return DiscUselessPeer
	}

	// Repeat the encryption handshake checks because the
	// peer set might have changed between the handshakes.  重复加密握手检查，因为对等组可能在握手之间发生了变化。
	return srv.encHandshakeChecks(peers, c)
}

func (srv *Server) encHandshakeChecks(peers map[discover.NodeID]*Peer, c *conn) error {
	switch {
	case !c.is(trustedConn|staticDialedConn) && len(peers) >= srv.MaxPeers:
		return DiscTooManyPeers
	case peers[c.id] != nil:
		return DiscAlreadyConnected
	case c.id == srv.Self().ID:
		return DiscSelf
	default:
		return nil
	}
}

type tempError interface {
	Temporary() bool
}

// listenLoop runs in its own goroutine and accepts
// inbound connections.  listenLoop 在自己的 goroutine 中运行并接受入站连接
func (srv *Server) listenLoop() {
	defer srv.loopWG.Done()
	glog.V(logger.Info).Infoln("Listening on", srv.listener.Addr())

	// This channel acts as a semaphore limiting
	// active inbound connections that are lingering pre-handshake.
	// If all slots are taken, no further connections are accepted.
	// 该通道充当信号量，限制在预握手中挥之不去的活动入站连接。
	// 如果所有插槽都被占用，则不接受进一步的连接。
	tokens := maxAcceptConns
	if srv.MaxPendingPeers > 0 {
		tokens = srv.MaxPendingPeers
	}
	slots := make(chan struct{}, tokens)
	for i := 0; i < tokens; i++ {
		slots <- struct{}{}
	}

	for {
		// Wait for a handshake slot before accepting.
		<-slots

		var (
			fd  net.Conn
			err error
		)
		for {
			fd, err = srv.listener.Accept()
			if tempErr, ok := err.(tempError); ok && tempErr.Temporary() {
				glog.V(logger.Debug).Infof("Temporary read error: %v", err)
				continue
			} else if err != nil {
				glog.V(logger.Debug).Infof("Read error: %v", err)
				return
			}
			break
		}
		fd = newMeteredConn(fd, true) //newMeteredConn 创建一个新的计量连接，同时增加入口或出口连接计量。 如果禁用度量系统，则此函数返回原始对象。
		//		glog.V(logger.Debug).Infof("fd nodetype %v\n", fd)
		glog.V(logger.Debug).Infof("Accepted conn %v\n", fd.RemoteAddr())
		//		glog.V(logger.Debug).Infof("Accepted conn %v\n", fd.RemoteAddr().Network())

		/*
		 ** TODO: we must get the nodetype
		 **
		 */

		// Spawn the handler. It will give the slot back when the connection
		// has been established. 生成处理程序。 建立连接后，它将返回插槽。
		go func() {
			srv.setupConn(fd, inboundConn, nil)
			slots <- struct{}{}
		}()
	}
}

// setupConn runs the handshakes and attempts to add the connection
// as a peer. It returns when the connection has been added as a peer
// or the handshakes have failed.  setupConn 运行握手并尝试将连接添加为对等点。 当连接被添加为对等方或握手失败时，它会返回。
func (srv *Server) setupConn(fd net.Conn, flags connFlag, dialDest *discover.Node) {
	// Prevent leftover pending conns from entering the handshake. 防止剩余的待处理 conns 进入握手
	srv.lock.Lock()
	running := srv.running
	srv.lock.Unlock()
	/*
	 ** 此处添加 dialDest 是否为 nil 的判断
	 */
	destnodetype := uint64(0) // 设置默认值
	if dialDest != nil {
		destnodetype = dialDest.NodeType
	}
	c := &conn{fd: fd, transport: srv.NewTransport(fd), flags: flags, cont: make(chan error), NodeType: destnodetype}
	if !running {
		c.close(errServerStopped)
		return
	}
	// Run the encryption handshake.  运行加密握手
	var err error
	if c.id, err = c.doEncHandshake(srv.PrivateKey, dialDest); err != nil {
		glog.V(logger.Debug).Infof("%v faild enc handshake: %v", c, err)
		c.close(err)
		return
	}
	// For dialed connections, check that the remote public key matches.  对于拨号连接，检查远程公钥是否匹配
	if dialDest != nil && c.id != dialDest.ID {
		c.close(DiscUnexpectedIdentity)
		glog.V(logger.Debug).Infof("%v dialed identity mismatch, want %x", c, dialDest.ID[:8])
		return
	}
	if err := srv.checkpoint(c, srv.posthandshake); err != nil {
		glog.V(logger.Debug).Infof("%v failed checkpoint posthandshake: %v", c, err)
		c.close(err)
		return
	}
	// Run the protocol handshake   运行协议握手
	phs, err := c.doProtoHandshake(srv.ourHandshake)
	if err != nil {
		glog.V(logger.Debug).Infof("%v failed proto handshake: %v", c, err)
		c.close(err)
		return
	}
	if phs.ID != c.id {
		glog.V(logger.Debug).Infof("%v wrong proto handshake identity: %x", c, phs.ID[:8])
		c.close(DiscUnexpectedIdentity)
		return
	}
	/*
	* TODO: 更新 conn peer 的 nodetype
	 */
	c.NodeType = phs.NodeType

	c.caps, c.name = phs.Caps, phs.Name
	if err := srv.checkpoint(c, srv.addpeer); err != nil {
		glog.V(logger.Debug).Infof("%v failed checkpoint addpeer: %v", c, err)
		c.close(err)
		return
	}
	// If the checks completed successfully, runPeer has now been
	// launched by run.如果检查成功完成，runPeer 现在已由 run 启动。
}

// checkpoint sends the conn to run, which performs the
// post-handshake checks for the stage (posthandshake, addpeer). checkpoint 发送 conn 运行，它执行阶段的握手后检查（posthandshake，addpeer）
func (srv *Server) checkpoint(c *conn, stage chan<- *conn) error {
	select {
	case stage <- c:
	case <-srv.quit:
		return errServerStopped
	}
	select {
	case err := <-c.cont:
		return err
	case <-srv.quit:
		return errServerStopped
	}
}

// runPeer runs in its own goroutine for each peer.
// it waits until the Peer logic returns and removes
// the peer.
// runPeer 在自己的 goroutine 中为每个 peer 运行。
// 它一直等到对等逻辑返回并删除对等点。
func (srv *Server) runPeer(p *Peer) {
	glog.V(logger.Debug).Infof("Added %v\n", p)
	srvjslog.LogJson(&logger.P2PConnected{
		RemoteId:            p.ID().String(),
		RemoteAddress:       p.RemoteAddr().String(),
		RemoteVersionString: p.Name(),
		NumConnections:      srv.PeerCount(),
	})

	if srv.NewPeerHook != nil {
		srv.NewPeerHook(p)
	}
	discreason := p.run()

	// Note: run waits for existing peers to be sent on srv.delpeer
	// before returning, so this send should not select on srv.quit.
	// 注意：run 在返回之前等待现有的对等点在 srv.delpeer 上发送，所以这个发送不应该在 srv.quit 上选择。
	srv.delpeer <- p

	glog.V(logger.Debug).Infof("Removed %v (%v)\n", p, discreason)
	srvjslog.LogJson(&logger.P2PDisconnected{
		RemoteId:       p.ID().String(),
		NumConnections: srv.PeerCount(),
	})

}

// NodeInfo represents a short summary of the information known about the host.  NodeInfo 表示有关主机的已知信息的简短摘要
type NodeInfo struct {
	ID    string `json:"id"`    // Unique node identifier (also the encryption key)  唯一节点标识符（也是加密密钥）
	Name  string `json:"name"`  // Name of the node, including client type, version, OS, custom data  节点名称，包括客户端类型、版本、操作系统、自定义数据
	Enode string `json:"enode"` // Enode URL for adding this peer from remote peers   用于从远程对等点添加此对等点的 Enode URL
	IP    string `json:"ip"`    // IP address of the node  节点的IP地址
	Ports struct {
		Discovery int `json:"discovery"` // UDP listening port for discovery protocol   发现协议的 UDP 监听端口
		Listener  int `json:"listener"`  // TCP listening port for RLPx   RLPx 的 TCP 侦听端口
	} `json:"ports"`
	ListenAddr string                 `json:"listenAddr"`
	Protocols  map[string]interface{} `json:"protocols"`
	NodeType   uint64                 `json:"nodetype"` // type of the node
}

// Info gathers and returns a collection of metadata known about the host.  Info 收集并返回已知主机的元数据集合
func (srv *Server) NodeInfo() *NodeInfo {
	node := srv.Self()

	// Gather and assemble the generic node infos
	info := &NodeInfo{
		Name:       srv.Name,
		Enode:      node.String(),
		ID:         node.ID.String(),
		IP:         node.IP.String(),
		ListenAddr: srv.ListenAddr,
		NodeType:   srv.Self().NodeType,
		Protocols:  make(map[string]interface{}),
	}
	info.Ports.Discovery = int(node.UDP)
	info.Ports.Listener = int(node.TCP)

	// Gather all the running protocol infos (only once per protocol type) 收集所有正在运行的协议信息（每种协议类型仅一次）
	for _, proto := range srv.Protocols {
		if _, ok := info.Protocols[proto.Name]; !ok {
			nodeInfo := interface{}("unknown")
			if query := proto.NodeInfo; query != nil {
				nodeInfo = proto.NodeInfo()
			}
			info.Protocols[proto.Name] = nodeInfo
		}
	}
	return info
}

func (srv *Server) ConsensusPeerCount() int {
	var count int
	count = 0
	select {
	case srv.peerOp <- func(ps map[discover.NodeID]*Peer) {
		for _, p := range ps {
			if p.NodeType == uint64(1) || p.NodeType == uint64(0) {
				count++
			}
			//			count = len(ps)
		}
	}:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return count
}

func (srv *Server) ConsensusPeers() map[string]string {
	//	pInfo := srv.PeersInfo()
	pInfo := srv.PeersInfoAddSelf()
	newmap := make(map[string]string)
	for _, p := range pInfo {
		newmap[p.ID] = "Active"
	}
	return newmap

}

// PeersInfo returns an array of metadata objects describing connected peers. PeersInfo 返回描述连接对等点的元数据对象数组。
func (srv *Server) PeersInfo() []*PeerInfo {
	// Gather all the generic and sub-protocol specific infos
	infos := make([]*PeerInfo, 0, srv.PeerCount())
	for _, peer := range srv.Peers() {
		if peer != nil {
			infos = append(infos, peer.Info())
		}
	}
	// Sort the result array alphabetically by node identifier
	for i := 0; i < len(infos); i++ {
		for j := i + 1; j < len(infos); j++ {
			if infos[i].ID > infos[j].ID {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}
	return infos
}

//-----------------------------------//
func (srv *Server) LockServer() {
	srv.lock.Lock()

}
func (srv *Server) UnLockServer() {
	srv.lock.Unlock()
}

/*
	将本节点的信息也添加进去,同时作为peer
*/
func (srv *Server) PeersInfoAddSelf() []*PeerInfo {
	// Gather all the generic and sub-protocol specific infos
	infos := make([]*PeerInfo, 0, srv.ConsensusPeerCount())
	//	glog.V(logger.Detail).Infof("srv.ConsensusPeerCount() %d", srv.ConsensusPeerCount())
	for _, peer := range srv.Peers() {
		if peer != nil {
			if peer.NodeType == uint64(1) || peer.NodeType == uint64(0) {
				infos = append(infos, peer.Info())
			}

		}
	}
	localpeer := &PeerInfo{
		ID: srv.NodeInfo().ID,
	}
	infos = append(infos, localpeer)
	// Sort the result array alphabetically by node identifier
	for i := 0; i < len(infos); i++ {
		for j := i + 1; j < len(infos); j++ {
			if infos[i].ID > infos[j].ID {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}
	return infos
}

func newkey() *sm2.PrivateKey {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic("couldn't generate key: " + err.Error())
	}
	return key
}

type testTransport struct {
	id discover.NodeID
	*rlpx

	closeErr error
}

func NewTestTransport(id discover.NodeID, fd net.Conn) transport {
	wrapped := newRLPX(fd).(*rlpx)
	wrapped.rw = newRLPXFrameRW(fd, secrets{
		MAC:        zero16,
		AES:        zero16,
		IngressMAC: sm3.NewKeccak256(),
		EgressMAC:  sm3.NewKeccak256(),
	})
	return &testTransport{id: id, rlpx: wrapped}
}
func newTestTransport(id discover.NodeID, fd net.Conn) transport {
	wrapped := newRLPX(fd).(*rlpx)
	wrapped.rw = newRLPXFrameRW(fd, secrets{
		MAC:        zero16,
		AES:        zero16,
		IngressMAC: sm3.NewKeccak256(),
		EgressMAC:  sm3.NewKeccak256(),
	})
	return &testTransport{id: id, rlpx: wrapped}
}

func (c *testTransport) doEncHandshake(prv *sm2.PrivateKey, dialDest *discover.Node) (discover.NodeID, error) {
	return c.id, nil
}

func (c *testTransport) doProtoHandshake(our *protoHandshake) (*protoHandshake, error) {
	return &protoHandshake{ID: c.id, Name: "test"}, nil
}

func (c *testTransport) close(err error) {
	c.rlpx.fd.Close()
	c.closeErr = err
}

func (srv *Server) FindAndChangeConnByNodeID(nodeid discover.NodeID, nodetype uint64) bool {
	if srv.ntab == nil {
		glog.V(logger.Detail).Infof("srv ntab is nil ")
	}
	err := srv.ntab.UpdateNode(nodeid, nodetype)

	if err == nil {
		return true
	} else {
		return false
	}

}
