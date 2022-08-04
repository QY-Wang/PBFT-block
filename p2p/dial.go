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

package p2p

import (
	"container/heap"
	"crypto/rand"
	"fmt"
	"net"
	"time"

	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/p2p/discover"
)

const (
	// This is the amount of time spent waiting in between
	// redialing a certain node. 这是在重拨某个节点之间等待的时间量。
	dialHistoryExpiration = 30 * time.Second

	// Discovery lookups are throttled and can only run
	// once every few seconds. 发现查找受到限制，只能每隔几秒运行一次
	lookupInterval = 4 * time.Second

	// Endpoint resolution is throttled with bounded backoff. 端点分辨率受到有界回退的限制
	initialResolveDelay = 60 * time.Second
	maxResolveDelay     = time.Hour
)

// dialstate schedules dials and discovery lookups.
// it get's a chance to compute new tasks on every iteration
// of the main loop in Server.run.
// dialstate 安排拨号和发现查找。
// 它有机会在 Server.run 的主循环的每次迭代中计算新任务

type dialstate struct {
	maxDynDials int
	ntab        discoverTable

	lookupRunning bool
	dialing       map[discover.NodeID]connFlag
	lookupBuf     []*discover.Node // current discovery lookup results 当前发现查找结果
	randomNodes   []*discover.Node // filled from Table 从表中填充
	static        map[discover.NodeID]*dialTask
	hist          *dialHistory
}

type discoverTable interface {
	Self() *discover.Node
	Close()
	Resolve(target discover.NodeID) *discover.Node
	Lookup(target discover.NodeID) []*discover.Node
	ReadRandomNodes([]*discover.Node) int
	UpdateNode(id discover.NodeID, nodetype uint64) error
}

// the dial history remembers recent dials. 拨号历史记录最近的拨号
type dialHistory []pastDial

// pastDial is an entry in the dial history. pastDial 是拨号历史中的一个条目
type pastDial struct {
	id  discover.NodeID
	exp time.Time
}

type task interface {
	Do(*Server)
}

// A dialTask is generated for each node that is dialed. Its
// fields cannot be accessed while the task is running. 为每个被拨号的节点生成一个 dialTask。 任务运行时无法访问其字段。
type dialTask struct {
	flags        connFlag
	dest         *discover.Node
	lastResolved time.Time
	resolveDelay time.Duration
	NodeType     uint64 // 增加节点类型 , 暂时没有用到
}

// discoverTask runs discovery table operations.
// Only one discoverTask is active at any time.
// discoverTask.Do performs a random lookup.
// discoverTask 运行发现表操作。
// 任何时候只有一个 discoverTask 处于活动状态。
// discoverTask.Do 执行随机查找
type discoverTask struct {
	results []*discover.Node
}

// A waitExpireTask is generated if there are no other tasks
// to keep the loop in Server.run ticking. 如果没有其他任务来保持 Server.run 中的循环滴答，则会生成一个 waitExpireTask
type waitExpireTask struct {
	time.Duration
}

func newDialState(static []*discover.Node, ntab discoverTable, maxdyn int) *dialstate {
	s := &dialstate{
		maxDynDials: maxdyn,
		ntab:        ntab,
		static:      make(map[discover.NodeID]*dialTask),
		dialing:     make(map[discover.NodeID]connFlag),
		randomNodes: make([]*discover.Node, maxdyn/2),
		hist:        new(dialHistory),
	}
	for _, n := range static {
		//		newconNode := &ConNode{
		//			CNode:     n,
		//			CNodeType: 0,
		//		}
		s.addStatic(n)

	}
	return s
}

//func (s *dialstate) addStatic(n *discover.Node) {
//	// This overwites the task instead of updating an existing
//	// entry, giving users the opportunity to force a resolve operation.
//	s.static[n.ID] = &dialTask{flags: staticDialedConn, dest: n}
//}

func (s *dialstate) addStatic(n *discover.Node) {
	// This overwites the task instead of updating an existing
	// entry, giving users the opportunity to force a resolve operation.
	//这会覆盖任务而不是更新现有条目，从而使用户有机会强制执行解析操作。
	s.static[n.ID] = &dialTask{flags: staticDialedConn, dest: n, NodeType: n.NodeType}
	fmt.Println("添加静态节点" + n.ID.String())
}

func (s *dialstate) newTasks(nRunning int, peers map[discover.NodeID]*Peer, now time.Time) []task {
	var newtasks []task
	isDialing := func(id discover.NodeID) bool {
		_, found := s.dialing[id]
		return found || peers[id] != nil || s.hist.contains(id)
	}
	addDial := func(flag connFlag, n *discover.Node) bool {
		if isDialing(n.ID) {
			return false
		}
		s.dialing[n.ID] = flag
		newtasks = append(newtasks, &dialTask{flags: flag, dest: n})
		return true
	}

	// Compute number of dynamic dials necessary at this point. 计算此时需要的动态拨号数
	needDynDials := s.maxDynDials
	for _, p := range peers {
		if p.rw.is(dynDialedConn) {
			needDynDials--
		}
	}
	for _, flag := range s.dialing {
		if flag&dynDialedConn != 0 {
			needDynDials--
		}
	}

	// Expire the dial history on every invocation. 使每次调用的拨号历史记录过期
	s.hist.expire(now)

	// Create dials for static nodes if they are not connected. 如果静态节点未连接，则为它们创建拨号
	for id, t := range s.static {
		if !isDialing(id) {
			s.dialing[id] = t.flags
			newtasks = append(newtasks, t)
		}
	}

	// Use random nodes from the table for half of the necessary
	// dynamic dials. 将表中的随机节点用于一半必要的动态拨号
	randomCandidates := needDynDials / 2
	if randomCandidates > 0 {
		n := s.ntab.ReadRandomNodes(s.randomNodes)
		for i := 0; i < randomCandidates && i < n; i++ {
			if addDial(dynDialedConn, s.randomNodes[i]) {
				needDynDials--
			}
		}
	}
	// Create dynamic dials from random lookup results, removing tried
	// items from the result buffer. 从随机查找结果创建动态拨号表，从结果缓冲区中删除尝试过的项目
	i := 0
	for ; i < len(s.lookupBuf) && needDynDials > 0; i++ {
		if addDial(dynDialedConn, s.lookupBuf[i]) {
			needDynDials--
		}
	}
	s.lookupBuf = s.lookupBuf[:copy(s.lookupBuf, s.lookupBuf[i:])]
	// Launch a discovery lookup if more candidates are needed. 如果需要更多候选者，则启动发现查找
	if len(s.lookupBuf) < needDynDials && !s.lookupRunning {
		s.lookupRunning = true
		newtasks = append(newtasks, &discoverTask{})
	}

	// Launch a timer to wait for the next node to expire if all
	// candidates have been tried and no task is currently active.
	// This should prevent cases where the dialer logic is not ticked
	// because there are no pending events.
	// 如果所有候选者都已尝试并且当前没有任务处于活动状态，则启动一个计时器以等待下一个节点到期。这应该可以防止由于没有未决事件而未勾选拨号逻辑的情况
	if nRunning == 0 && len(newtasks) == 0 && s.hist.Len() > 0 {
		t := &waitExpireTask{s.hist.min().exp.Sub(now)}
		newtasks = append(newtasks, t)
	}
	return newtasks
}

func (s *dialstate) taskDone(t task, now time.Time) {
	switch t := t.(type) {
	case *dialTask:
		s.hist.add(t.dest.ID, now.Add(dialHistoryExpiration))
		delete(s.dialing, t.dest.ID)
	case *discoverTask:
		s.lookupRunning = false
		s.lookupBuf = append(s.lookupBuf, t.results...)
	}
}

func (t *dialTask) Do(srv *Server) {
	if t.dest.Incomplete() {
		if !t.resolve(srv) {
			return
		}
	}
	success := t.dial(srv, t.dest)
	// Try resolving the ID of static nodes if dialing failed. 如果拨号失败，请尝试解析静态节点的 ID。
	if !success && t.flags&staticDialedConn != 0 {
		if t.resolve(srv) {
			t.dial(srv, t.dest)
		}
	}
}

// resolve attempts to find the current endpoint for the destination
// using discovery.
//
// Resolve operations are throttled with backoff to avoid flooding the
// discovery network with useless queries for nodes that don't exist.
// The backoff delay resets when the node is found.
// resolve 尝试使用发现来查找目标的当前端点。
// Resolve 操作通过退避来限制，以避免对不存在的节点的无用查询淹没发现网络。退避延迟在找到节点时重置。
func (t *dialTask) resolve(srv *Server) bool {
	if srv.ntab == nil {
		glog.V(logger.Debug).Infof("can't resolve node %x: discovery is disabled", t.dest.ID[:6])
		return false
	}
	if t.resolveDelay == 0 {
		t.resolveDelay = initialResolveDelay
	}
	if time.Since(t.lastResolved) < t.resolveDelay {
		return false
	}
	resolved := srv.ntab.Resolve(t.dest.ID)
	t.lastResolved = time.Now()
	if resolved == nil {
		t.resolveDelay *= 2
		if t.resolveDelay > maxResolveDelay {
			t.resolveDelay = maxResolveDelay
		}
		glog.V(logger.Debug).Infof("resolving node %x failed (new delay: %v)", t.dest.ID[:6], t.resolveDelay)
		return false
	}
	// The node was found.
	t.resolveDelay = initialResolveDelay
	t.dest = resolved
	glog.V(logger.Debug).Infof("resolved node %x: %v:%d", t.dest.ID[:6], t.dest.IP, t.dest.TCP)
	return true
}

// dial performs the actual connection attempt. dial 执行实际的连接尝试
func (t *dialTask) dial(srv *Server, dest *discover.Node) bool {
	addr := &net.TCPAddr{IP: dest.IP, Port: int(dest.TCP)}
	glog.V(logger.Debug).Infof("dial tcp %v (%x)\n", addr, dest.ID[:6])
	fd, err := srv.Dialer.Dial("tcp", addr.String())
	if err != nil {
		glog.V(logger.Detail).Infof("%v", err)
		return false
	}
	mfd := newMeteredConn(fd, false)
	srv.setupConn(mfd, t.flags, dest)
	return true
}

func (t *dialTask) String() string {
	return fmt.Sprintf("%v %x %v:%d", t.flags, t.dest.ID[:8], t.dest.IP, t.dest.TCP)
}

func (t *discoverTask) Do(srv *Server) {
	// newTasks generates a lookup task whenever dynamic dials are
	// necessary. Lookups need to take some time, otherwise the
	// event loop spins too fast. 每当需要动态拨号时，newTasks 都会生成一个查找任务。 查找需要一些时间，否则事件循环旋转得太快。
	next := srv.lastLookup.Add(lookupInterval)
	if now := time.Now(); now.Before(next) {
		time.Sleep(next.Sub(now))
	}
	srv.lastLookup = time.Now()
	var target discover.NodeID
	rand.Read(target[:])
	t.results = srv.ntab.Lookup(target)
}

func (t *discoverTask) String() string {
	s := "discovery lookup"
	if len(t.results) > 0 {
		s += fmt.Sprintf(" (%d results)", len(t.results))
	}
	return s
}

func (t waitExpireTask) Do(*Server) {
	time.Sleep(t.Duration)
}
func (t waitExpireTask) String() string {
	return fmt.Sprintf("wait for dial hist expire (%v)", t.Duration)
}

// Use only these methods to access or modify dialHistory. 仅使用这些方法来访问或修改 dialHistory
func (h dialHistory) min() pastDial {
	return h[0]
}
func (h *dialHistory) add(id discover.NodeID, exp time.Time) {
	heap.Push(h, pastDial{id, exp})
}
func (h dialHistory) contains(id discover.NodeID) bool {
	for _, v := range h {
		if v.id == id {
			return true
		}
	}
	return false
}
func (h *dialHistory) expire(now time.Time) {
	for h.Len() > 0 && h.min().exp.Before(now) {
		heap.Pop(h)
	}
}

// heap.Interface boilerplate heap.Interface 样板
func (h dialHistory) Len() int           { return len(h) }
func (h dialHistory) Less(i, j int) bool { return h[i].exp.Before(h[j].exp) }
func (h dialHistory) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *dialHistory) Push(x interface{}) {
	*h = append(*h, x.(pastDial))
}
func (h *dialHistory) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
