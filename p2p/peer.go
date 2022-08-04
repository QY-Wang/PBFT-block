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

package p2p

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/p2p/discover"
	"github.com/DWBC-ConPeer/rlp"
)

const (
	baseProtocolVersion    = 4
	baseProtocolLength     = uint64(17) // 之前是16， //20190823 pbft的7中msg合并为PbftMsg
	baseProtocolMaxMsgSize = 2 * 1024

	pingInterval = 15 * time.Second
)

const (
	// devp2p message codes
	handshakeMsg = 0x00
	discMsg      = 0x01
	pingMsg      = 0x02
	pongMsg      = 0x03
	getPeersMsg  = 0x04
	peersMsg     = 0x05
)

const (
	PingMsg = 0x02
)

// protoHandshake is the RLP structure of the protocol handshake. protoHandshake 是协议握手的 RLP 结构
type protoHandshake struct {
	Version    uint64
	Name       string
	Caps       []Cap
	ListenPort uint64
	ID         discover.NodeID
	/*
	* TODO: 附带本节点的节点类型
	* nodetype 均为固定的
	 */
	NodeType uint64
	// Ignore additional fields (for forward compatibility). 忽略其他字段（为了向前兼容）
	Rest []rlp.RawValue `rlp:"tail"`
}

// Peer represents a connected remote node. Peer 代表一个连接的远程节点
type Peer struct {
	rw      *conn
	running map[string]*protoRW
	//WaitGroup 等待一组 goroutine 完成。
	// 主 goroutine 调用 Add 来设置要等待的 goroutine 的数量。
	//然后每个 goroutine 运行并在完成时调用 Done。 同时，Wait 可以用来阻塞，直到所有的 goroutine 都完成。
	//首次使用后不得复制 WaitGroup。
	wg       sync.WaitGroup
	protoErr chan error
	closed   chan struct{}
	disc     chan DiscReason
	NodeType uint64 // 节点类型
	NodePK   []byte //节点公钥

}

func (p *Peer) GetConn() *conn {
	return p.rw
}

// NewPeer returns a peer for testing purposes.  NewPeer 返回一个对等点以进行测试
func NewPeer(id discover.NodeID, name string, caps []Cap) *Peer {
	pipe, _ := net.Pipe()
	conn := &conn{fd: pipe, transport: nil, id: id, caps: caps, name: name}
	peer := newPeer(conn, nil)
	close(peer.closed) // ensures Disconnect doesn't block  确保断开连接不会阻塞
	return peer
}

// NewPeerByType returns a peer for testing purposes.
func NewPeerByType(id discover.NodeID, name string, caps []Cap, nodetype uint64) *Peer {
	pipe, _ := net.Pipe()
	conn := &conn{fd: pipe, transport: nil, id: id, caps: caps, name: name, NodeType: nodetype}
	peer := newPeer(conn, nil)
	close(peer.closed) // ensures Disconnect doesn't block
	return peer
}

// ID returns the node's public key. ID 返回节点的公钥
func (p *Peer) ID() discover.NodeID {
	return p.rw.id
}

// Name returns the node name that the remote node advertised. Name 返回远程节点通告的节点名称
func (p *Peer) Name() string {
	return p.rw.name
}

// Caps returns the capabilities (supported subprotocols) of the remote peer. Caps 返回远程对等点的功能（支持的子协议）
func (p *Peer) Caps() []Cap {
	// TODO: maybe return copy
	return p.rw.caps
}

// RemoteAddr returns the remote address of the network connection.
//RemoteAddr 返回网络连接的远程地址
func (p *Peer) RemoteAddr() net.Addr {
	return p.rw.fd.RemoteAddr()
}

// LocalAddr returns the local address of the network connection. LocalAddr 返回网络连接的本地地址
func (p *Peer) LocalAddr() net.Addr {
	return p.rw.fd.LocalAddr()
}

// Disconnect terminates the peer connection with the given reason.
// It returns immediately and does not wait until the connection is closed.
//Disconnect 以给定的原因终止对等连接。
//它立即返回并且不等到连接关闭。
func (p *Peer) Disconnect(reason DiscReason) {
	select {
	case p.disc <- reason:
	case <-p.closed:
	}
}

// String implements fmt.Stringer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer %x %v", p.rw.id[:8], p.RemoteAddr())
}

/*
** 声明：conn的nodetyoe属性以默认赋值
 */
func newPeer(conn *conn, protocols []Protocol) *Peer {
	/*
	* 增加节点类型判断
	 */
	if conn.NodeType == uint64(4) {

		protomap := matchProtocols(protocols, conn.caps, conn)
		p := &Peer{
			rw:       conn,
			running:  protomap,
			disc:     make(chan DiscReason),
			protoErr: make(chan error, len(protomap)+1), // protocols + pingLoop
			closed:   make(chan struct{}),
			NodeType: conn.NodeType,
		}
		return p
	} else {
		protomap := matchProtocols(protocols, conn.caps, conn)
		p := &Peer{
			rw:       conn,
			running:  protomap,
			disc:     make(chan DiscReason),
			protoErr: make(chan error, len(protomap)+1), // protocols + pingLoop
			closed:   make(chan struct{}),
			NodeType: conn.NodeType,
		}
		return p

	}

}

// called by server.go/runPeer
func (p *Peer) run() DiscReason {
	var (
		writeStart = make(chan struct{}, 1)
		writeErr   = make(chan error, 1)
		readErr    = make(chan error, 1)
		reason     DiscReason
		requested  bool
	)
	p.wg.Add(2)
	go p.readLoop(readErr)
	go p.pingLoop()

	// Start all protocol handlers.
	writeStart <- struct{}{}
	p.startProtocols(writeStart, writeErr)

	// Wait for an error or disconnect.
loop:
	for {
		select {
		case err := <-writeErr:
			// A write finished. Allow the next write to start if
			// there was no error. 写完了。 如果没有错误，则允许下一次写入开始
			if err != nil {
				glog.V(logger.Detail).Infof("%v: write error: %v\n", p, err)
				reason = DiscNetworkError
				break loop
			}
			writeStart <- struct{}{}
		case err := <-readErr:
			if r, ok := err.(DiscReason); ok {
				glog.V(logger.Debug).Infof("%v: remote requested disconnect: %v\n", p, r)
				requested = true
				reason = r
			} else {
				glog.V(logger.Detail).Infof("%v: read error: %v\n", p, err)
				reason = DiscNetworkError
			}
			break loop
		case err := <-p.protoErr:
			/*
			** 节点类型判断
			 */
			if p.NodeType != uint64(4) {
				reason = discReasonForError(err)
				glog.V(logger.Debug).Infof("%v: protocol error: %v (%v)\n", p, err, reason)
				break loop
			}

		case reason = <-p.disc:
			glog.V(logger.Debug).Infof("%v: locally requested disconnect: %v\n", p, reason)
			break loop
		}
	}

	close(p.closed)
	p.rw.close(reason)
	p.wg.Wait()
	if requested {
		reason = DiscRequested
	}
	return reason
}

func (p *Peer) pingLoop() {
	ping := time.NewTicker(pingInterval) //NewTicker 返回一个新的 Ticker，其中包含一个通道，该通道将在每次滴答后发送通道上的时间。
	//刻度的周期由持续时间参数指定。 自动收报机将调整时间间隔或丢弃滴答声以弥补慢速接收器。 持续时间 d 必须大于零；
	//如果没有，NewTicker 会恐慌。 停止代码以释放相关资源。
	defer p.wg.Done()
	defer ping.Stop()
	for {
		select {
		case <-ping.C:
			if err := SendItems(p.rw, pingMsg); err != nil {
				p.protoErr <- err
				return
			}
		case <-p.closed:
			return
		}
	}
}

func (p *Peer) readLoop(errc chan<- error) {
	defer p.wg.Done()
	for {
		msg, err := p.rw.ReadMsg()
		if err != nil {
			errc <- err
			return
		}
		msg.ReceivedAt = time.Now()
		if err = p.handle(msg); err != nil {
			errc <- err
			return
		}
	}
}

func (p *Peer) handle(msg Msg) error {
	switch {
	case msg.Code == pingMsg:
		msg.Discard() //Discard 将任何剩余的有效载荷数据读入黑洞
		go SendItems(p.rw, pongMsg)
	case msg.Code == discMsg:
		var reason [1]DiscReason
		// This is the last message. We don't need to discard or
		// check errors because, the connection will be closed after it.这是最后一条消息。 我们不需要丢弃或检查错误，因为连接将在之后关闭
		rlp.Decode(msg.Payload, &reason)
		return reason[0]
	case msg.Code < baseProtocolLength:
		// ignore other base protocol messages 忽略其他基本协议消息
		return msg.Discard()
	default:
		//		glog.V(logger.Info).Infof("proto msg.code=%d", msg.Code)
		// it's a subprotocol message 这是一个子协议消息
		proto, err := p.getProto(msg.Code) //getProto 查找负责处理给定消息代码的协议。
		if err != nil {
			return fmt.Errorf("msg code out of range: %v", msg.Code)
		}

		select {
		case proto.in <- msg:
			return nil
		case <-p.closed:
			return io.EOF
		}
	}
	return nil
}

func countMatchingProtocols(protocols []Protocol, caps []Cap) int {
	n := 0
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				n++
			}
		}
	}
	return n
}

// matchProtocols creates structures for matching named subprotocols.  matchProtocols 创建用于匹配命名子协议的结构
func matchProtocols(protocols []Protocol, caps []Cap, rw MsgReadWriter) map[string]*protoRW {
	sort.Sort(capsByNameAndVersion(caps))
	offset := baseProtocolLength
	result := make(map[string]*protoRW)
	//	glog.V(logger.Info).Infoln("caps %d", caps)
outer:
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				// If an old protocol version matched, revert it 如果旧协议版本匹配，则将其还原
				if old := result[cap.Name]; old != nil {
					offset -= old.Length
				}
				// Assign the new match
				result[cap.Name] = &protoRW{Protocol: proto, offset: offset, in: make(chan Msg), w: rw}
				offset += proto.Length

				continue outer
			}
		}
	}
	//	glog.V(logger.Info).Infoln("result %d", result)
	return result
}

// matchProtocols creates structures for matching named subprotocols. For nodetype 4  matchProtocols 创建用于匹配命名子协议的结构。对于节点类型 4
func matchProtocolsForMovingNode(protocols []Protocol, caps []Cap, rw MsgReadWriter) map[string]*protoRW {
	sort.Sort(capsByNameAndVersion(caps))
	offset := baseProtocolLength
	result := make(map[string]*protoRW)
	glog.V(logger.Info).Infoln("caps %d", caps)
outer:
	//	for _, cap := range caps {
	for _, proto := range protocols {
		if true {
			// If an old protocol version matched, revert it
			//				if old := result[cap.Name]; old != nil {
			//					offset -= old.Length
			//				}
			// Assign the new match
			result[proto.Name] = &protoRW{Protocol: proto, offset: offset, in: make(chan Msg), w: rw}
			offset += proto.Length

			continue outer
		}
		//		}
	}
	return result
}

func (p *Peer) startProtocols(writeStart <-chan struct{}, writeErr chan<- error) {
	p.wg.Add(len(p.running))
	for _, proto := range p.running {
		proto := proto
		proto.closed = p.closed
		proto.wstart = writeStart
		proto.werr = writeErr
		glog.V(logger.Detail).Infof("%v: Starting protocol %s/%d\n", p, proto.Name, proto.Version)
		go func() {
			err := proto.Run(p, proto) //当与对等方协商协议时，在新的 groutine 中调用 run。它应该从 rw 读取和写入消息。每个消息的有效负载必须完全消耗。
			//对等连接在 Start 返回时关闭。它应该返回遇到的任何协议级错误（例如 I/O 错误）
			if err == nil {
				glog.V(logger.Detail).Infof("%v: Protocol %s/%d returned\n", p, proto.Name, proto.Version)
				err = errors.New("protocol returned")
			} else if err != io.EOF {
				glog.V(logger.Detail).Infof("%v: Protocol %s/%d error: %v\n", p, proto.Name, proto.Version, err)
			}
			//			glog.V(logger.Detail).Infof("err %d ", err)
			p.protoErr <- err
			p.wg.Done()
		}()
	}
}

// getProto finds the protocol responsible for handling
// the given message code.
// getProto 找到负责处理给定消息代码的协议
func (p *Peer) getProto(code uint64) (*protoRW, error) {
	for _, proto := range p.running {
		if code >= proto.offset && code < proto.offset+proto.Length {
			return proto, nil
		}
	}
	return nil, newPeerError(errInvalidMsgCode, "%d", code)
}

type protoRW struct {
	Protocol
	in     chan Msg        // receices read messages  收据阅读信息
	closed <-chan struct{} // receives when peer is shutting down 当对等体关闭时接收
	wstart <-chan struct{} // receives when write may start  接收何时开始写入
	werr   chan<- error    // for write results  用于写入结果
	offset uint64
	w      MsgWriter
}

func (rw *protoRW) WriteMsg(msg Msg) (err error) {
	//	glog.V(logger.Info).Infof("protoRW WriteMsg length=%d, msg.Code=%d ", rw.Length, msg.Code)
	if msg.Code >= rw.Length {
		return newPeerError(errInvalidMsgCode, "not handled")
	}

	msg.Code += rw.offset

	select {
	case <-rw.wstart:
		err = rw.w.WriteMsg(msg)
		// Report write status back to Peer.run. It will initiate
		// shutdown if the error is non-nil and unblock the next write
		// otherwise. The calling protocol code should exit for errors
		// as well but we don't want to rely on that. 将写入状态报告给 Peer.run。如果错误为非 nil，它将启动关闭，否则取消阻止下一次写入。调用协议代码也应该退出错误，但我们不想依赖它
		if err != nil {
			glog.V(logger.Info).Infof("protoRW WriteMsg err=%d ", err)
		}

		rw.werr <- err
	case <-rw.closed:
		err = fmt.Errorf("shutting down")
	}
	return err
}

func (rw *protoRW) ReadMsg() (Msg, error) {
	select {
	case msg := <-rw.in:
		msg.Code -= rw.offset
		//		glog.V(logger.Info).Infof("protoRW ReadMsg  msg.Code=%d ", msg.Code)

		return msg, nil
	case <-rw.closed:
		return Msg{}, io.EOF
	}
}

// PeerInfo represents a short summary of the information known about a connected
// peer. Sub-protocol independent fields are contained and initialized here, with
// protocol specifics delegated to all connected sub-protocols. PeerInfo 表示有关已连接对等点的已知信息的简短摘要。 子协议独立字段包含并在这里初始化，协议细节委托给所有连接的子协议
type PeerInfo struct {
	ID      string   `json:"id"`   // Unique node identifier (also the encryption key)
	Name    string   `json:"name"` // Name of the node, including client type, version, OS, custom data
	Caps    []string `json:"caps"` // Sum-protocols advertised by this particular peer 此特定对等方通告的总和协议
	Network struct {
		LocalAddress string `json:"localAddress"` // Local endpoint of the TCP data connection

		RemoteAddress string `json:"remoteAddress"` // Remote endpoint of the TCP data connection
	} `json:"network"`
	Protocols map[string]interface{} `json:"protocols"` // Sub-protocol specific metadata fields  子协议特定的元数据字段
	NodeType  uint64                 `json:"NodeType"`
}

// Info gathers and returns a collection of metadata known about a peer.
func (p *Peer) Info() *PeerInfo {
	// Gather the protocol capabilities
	var caps []string
	for _, cap := range p.Caps() {
		caps = append(caps, cap.String())
	}
	// Assemble the generic peer metadata
	info := &PeerInfo{
		ID:        p.ID().String(),
		Name:      p.Name(),
		Caps:      caps,
		Protocols: make(map[string]interface{}),
		NodeType:  p.NodeType,
	}
	info.Network.LocalAddress = p.LocalAddr().String()
	info.Network.RemoteAddress = p.RemoteAddr().String()

	// Gather all the running protocol infos
	for _, proto := range p.running {
		protoInfo := interface{}("unknown")
		if query := proto.Protocol.PeerInfo; query != nil {
			if metadata := query(p.ID()); metadata != nil {
				protoInfo = metadata
			} else {
				protoInfo = "handshake"
			}
		}
		info.Protocols[proto.Name] = protoInfo
	}
	return info
}
