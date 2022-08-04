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

// Package discover implements the Node Discovery Protocol.
//
// The Node Discovery protocol provides a way to find RLPx nodes that
// can be connected to. It uses a Kademlia-like protocol to maintain a
// distributed database of the IDs and endpoints of all listening
// nodes.
package discover

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/DWBC-ConPeer/common"
	crypto "github.com/DWBC-ConPeer/crypto-gm"
	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
)

const (
	alpha      = 3  // Kademlia concurrency factor
	bucketSize = 16 // Kademlia bucket size
	hashBits   = len(common.Hash{}) * 8
	nBuckets   = hashBits + 1 // Number of buckets

	maxBondingPingPongs = 16
	maxFindnodeFailures = 5

	autoRefreshInterval = 1 * time.Hour
	seedCount           = 30
	seedMaxAge          = 5 * 24 * time.Hour
)

type Table struct {
	mutex   sync.Mutex        // protects buckets, their content, and nursery  保护存储桶、它们的内容和托儿所
	buckets [nBuckets]*bucket // index of known nodes by distance
	nursery []*Node           // bootstrap nodes
	db      *nodeDB           // database of known nodes 已知节点数据库

	refreshReq chan chan struct{}
	closeReq   chan struct{}
	closed     chan struct{}

	bondmu    sync.Mutex
	bonding   map[NodeID]*bondproc
	bondslots chan struct{} // limits total number of active bonding processes 请求绑定槽以限制网络使用

	nodeAddedHook func(*Node) // for testing

	net  transport
	self *Node // metadata of the local node
	/*
	* 当setSelfNodeType函数执行时，需调用此函数修改self(local node)的nodetype
	 */
}

type bondproc struct {
	err  error
	n    *Node
	done chan struct{}
}

// transport is implemented by the UDP transport.
// it is an interface so we can test without opening lots of UDP
// sockets and without generating a private key.
// 传输由UDP传输实现。
// 它是一个接口，因此我们可以在不打开大量 UDP 套接字且不生成私钥的情况下进行测试。
type transport interface {
	ping(NodeID, *net.UDPAddr) error
	waitping(NodeID) error
	findnode(toid NodeID, addr *net.UDPAddr, target NodeID) ([]*Node, error)
	close()
}

// bucket contains nodes, ordered by their last activity. the entry
// that was most recently active is the first element in entries.
type bucket struct{ entries []*Node }

func newTable(t transport, ourID NodeID, ourAddr *net.UDPAddr, nodeDBPath string) (*Table, error) {
	// If no node database was given, use an in-memory one 如果没有给出节点数据库，则使用内存中的数据库
	db, err := newNodeDB(nodeDBPath, Version, ourID)
	if err != nil {
		return nil, err
	}
	tab := &Table{
		net:        t,
		db:         db,
		self:       NewNode(ourID, ourAddr.IP, uint16(ourAddr.Port), uint16(ourAddr.Port)),
		bonding:    make(map[NodeID]*bondproc),
		bondslots:  make(chan struct{}, maxBondingPingPongs),
		refreshReq: make(chan chan struct{}),
		closeReq:   make(chan struct{}),
		closed:     make(chan struct{}),
	}
	for i := 0; i < cap(tab.bondslots); i++ {
		tab.bondslots <- struct{}{}
	}
	for i := range tab.buckets {
		tab.buckets[i] = new(bucket)
	}
	go tab.refreshLoop()
	return tab, nil
}

// Self returns the local node.
// The returned node should not be modified by the caller.
func (tab *Table) Self() *Node {
	return tab.self
}

// ReadRandomNodes fills the given slice with random nodes from the
// table. It will not write the same node more than once. The nodes in
// the slice are copies and can be modified by the caller.
// ReadRandomNodes 用表中的随机节点填充给定的切片。它不会多次写入同一个节点。切片中的节点是副本，可以由调用者修改。
func (tab *Table) ReadRandomNodes(buf []*Node) (n int) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	// TODO: tree-based buckets would help here
	// Find all non-empty buckets and get a fresh slice of their entries.
	// TODO：基于树的存储桶在这里会有所帮助
	// 找到所有非空桶并获取它们的新条目。
	var buckets [][]*Node
	for _, b := range tab.buckets {
		if len(b.entries) > 0 {
			buckets = append(buckets, b.entries[:])
		}
	}
	if len(buckets) == 0 {
		return 0
	}
	// Shuffle the buckets. 洗牌
	for i := uint32(len(buckets)) - 1; i > 0; i-- {
		j := randUint(i)
		buckets[i], buckets[j] = buckets[j], buckets[i]
	}
	// Move head of each bucket into buf, removing buckets that become empty. 将每个桶的头部移动到 buf 中，删除变为空的桶。
	var i, j int
	for ; i < len(buf); i, j = i+1, (j+1)%len(buckets) {
		b := buckets[j]
		buf[i] = &(*b[0])
		buckets[j] = b[1:]
		if len(b) == 1 {
			buckets = append(buckets[:j], buckets[j+1:]...)
		}
		if len(buckets) == 0 {
			break
		}
	}
	return i + 1
}

func randUint(max uint32) uint32 {
	if max == 0 {
		return 0
	}
	var b [4]byte
	rand.Read(b[:])
	return binary.BigEndian.Uint32(b[:]) % max
}

// Close terminates the network listener and flushes the node database. Close 终止网络侦听器并刷新节点数据库。
func (tab *Table) Close() {
	select {
	case <-tab.closed:
		// already closed.
	case tab.closeReq <- struct{}{}:
		<-tab.closed // wait for refreshLoop to end.
	}
}

// SetFallbackNodes sets the initial points of contact. These nodes
// are used to connect to the network if the table is empty and there
// are no known nodes in the database.
// SetFallbackNodes 设置初始接触点，如果表为空且数据库中没有已知节点，这些节点用于连接网络。
func (tab *Table) SetFallbackNodes(nodes []*Node) error {
	for _, n := range nodes {
		if err := n.validateComplete(); err != nil {
			return fmt.Errorf("bad bootstrap/fallback node %q (%v)", n, err)
		}
	}
	tab.mutex.Lock()
	tab.nursery = make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		cpy := *n
		// Recompute cpy.sha because the node might not have been
		// created by NewNode or ParseNode. 重新计算 cpy.sha，因为该节点可能不是由 NewNode 或 ParseNode 创建的。
		cpy.sha = crypto.Keccak256Hash(n.ID[:])
		tab.nursery = append(tab.nursery, &cpy)
	}
	tab.mutex.Unlock()
	tab.refresh()
	return nil
}

// Resolve searches for a specific node with the given ID.
// It returns nil if the node could not be found.
// 解析搜索具有给定 ID 的特定节点。
// 如果找不到节点，则返回 nil。
func (tab *Table) Resolve(targetID NodeID) *Node {
	// If the node is present in the local table, no
	// network interaction is required. 如果节点存在于本地表中，则不需要网络交互。
	hash := crypto.Keccak256Hash(targetID[:])
	tab.mutex.Lock()
	cl := tab.closest(hash, 1)
	tab.mutex.Unlock()
	if len(cl.entries) > 0 && cl.entries[0].ID == targetID {
		return cl.entries[0]
	}
	// Otherwise, do a network lookup.
	result := tab.Lookup(targetID)
	for _, n := range result {
		if n.ID == targetID {
			return n
		}
	}
	return nil
}

// Lookup performs a network search for nodes close
// to the given target. It approaches the target by querying
// nodes that are closer to it on each iteration.
// The given target does not need to be an actual node
// identifier. Lookup 对靠近给定目标的节点执行网络搜索。它通过在每次迭代中查询更接近目标的节点来接近目标。给定目标不需要是实际的节点标识符。
func (tab *Table) Lookup(targetID NodeID) []*Node {
	return tab.lookup(targetID, true)
}

func (tab *Table) lookup(targetID NodeID, refreshIfEmpty bool) []*Node {
	var (
		target         = crypto.Keccak256Hash(targetID[:])
		asked          = make(map[NodeID]bool)
		seen           = make(map[NodeID]bool)
		reply          = make(chan []*Node, alpha)
		pendingQueries = 0
		result         *nodesByDistance
	)
	// don't query further if we hit ourself.
	// unlikely to happen often in practice.
	// 如果我们击中自己，请不要进一步询问。
	// 在实践中不太可能经常发生。
	asked[tab.self.ID] = true

	for {
		tab.mutex.Lock()
		// generate initial result set
		result = tab.closest(target, bucketSize)
		tab.mutex.Unlock()
		if len(result.entries) > 0 || !refreshIfEmpty {
			break
		}
		// The result set is empty, all nodes were dropped, refresh.
		// We actually wait for the refresh to complete here. The very
		// first query will hit this case and run the bootstrapping
		// logic.
		// 结果集为空，所有节点都被删除，刷新。
		// 我们实际上在这里等待刷新完成。
		// 第一个查询将遇到这种情况并运行引导逻辑。
		<-tab.refresh()
		refreshIfEmpty = false
	}

	for {
		// ask the alpha closest nodes that we haven't asked yet 询问我们尚未询问的最接近 alpha 的节点
		for i := 0; i < len(result.entries) && pendingQueries < alpha; i++ {
			n := result.entries[i]
			if !asked[n.ID] {
				asked[n.ID] = true
				pendingQueries++
				go func() {
					// Find potential neighbors to bond with 寻找潜在的邻居来建立联系
					r, err := tab.net.findnode(n.ID, n.addr(), targetID)
					if err != nil {
						// Bump the failure counter to detect and evacuate non-bonded entries 碰撞故障计数器以检测和疏散非绑定条目
						fails := tab.db.findFails(n.ID) + 1
						tab.db.updateFindFails(n.ID, fails)
						glog.V(logger.Detail).Infof("Bumping failures for %x: %d", n.ID[:8], fails)

						if fails >= maxFindnodeFailures {
							glog.V(logger.Detail).Infof("Evacuating node %x: %d findnode failures", n.ID[:8], fails)
							tab.delete(n)
						}
					}
					reply <- tab.bondall(r)
				}()
			}
		}
		if pendingQueries == 0 {
			// we have asked all closest nodes, stop the search 我们已经询问了所有最近的节点，停止搜索
			break
		}
		// wait for the next reply 等待下一个回复
		for _, n := range <-reply {
			if n != nil && !seen[n.ID] {
				seen[n.ID] = true
				result.push(n, bucketSize)
			}
		}
		pendingQueries--
	}
	return result.entries
}

func (tab *Table) refresh() <-chan struct{} {
	done := make(chan struct{})
	select {
	case tab.refreshReq <- done:
	case <-tab.closed:
		close(done)
	}
	return done
}

// refreshLoop schedules doRefresh runs and coordinates shutdown.refreshLoop 安排 doRefresh 运行并协调关闭
func (tab *Table) refreshLoop() {
	var (
		timer   = time.NewTicker(autoRefreshInterval)
		waiting []chan struct{} // accumulates waiting callers while doRefresh runs 在 doRefresh 运行时累积等待的调用者
		done    chan struct{}   // where doRefresh reports completion doRefresh 报告完成的地方
	)
loop:
	for {
		select {
		case <-timer.C:
			if done == nil {
				done = make(chan struct{})
				go tab.doRefresh(done)
			}
		case req := <-tab.refreshReq:
			waiting = append(waiting, req)
			if done == nil {
				done = make(chan struct{})
				go tab.doRefresh(done)
			}
		case <-done:
			for _, ch := range waiting {
				close(ch)
			}
			waiting = nil
			done = nil
		case <-tab.closeReq:
			break loop
		}
	}

	if tab.net != nil {
		tab.net.close()
	}
	if done != nil {
		<-done
	}
	for _, ch := range waiting {
		close(ch)
	}
	tab.db.close()
	close(tab.closed)
}

// doRefresh performs a lookup for a random target to keep buckets
// full. seed nodes are inserted if the table is empty (initial
// bootstrap or discarded faulty peers). doRefresh 对随机目标执行查找以保持存储桶已满。 如果表为空（初始引导程序或丢弃的故障对等方），则插入种子节点。
func (tab *Table) doRefresh(done chan struct{}) {
	defer close(done)

	// The Kademlia paper specifies that the bucket refresh should
	// perform a lookup in the least recently used bucket. We cannot
	// adhere to this because the findnode target is a 512bit value
	// (not hash-sized) and it is not easily possible to generate a
	// sha3 preimage that falls into a chosen bucket.
	// We perform a lookup with a random target instead.
	// Kademlia 论文指定存储桶刷新应该在最近最少使用的存储桶中执行查找。 我们不能坚持这一点，因为 findnode 目标是一个 512 位的值（不是散列大小的），并且不容易生成落入所选存储桶的 sha3 原像。
	// 我们使用随机目标执行查找。
	var target NodeID
	rand.Read(target[:])
	result := tab.lookup(target, false)
	if len(result) > 0 {
		return
	}

	// The table is empty. Load nodes from the database and insert
	// them. This should yield a few previously seen nodes that are
	// (hopefully) still alive. 空的。 从数据库中加载节点并插入它们。 这应该会产生一些以前看到的节点（希望）仍然活着。
	seeds := tab.db.querySeeds(seedCount, seedMaxAge)
	seeds = tab.bondall(append(seeds, tab.nursery...))
	if glog.V(logger.Debug) {
		if len(seeds) == 0 {
			glog.Infof("no seed nodes found")
		}
		for _, n := range seeds {
			age := time.Since(tab.db.lastPong(n.ID))
			glog.Infof("seed node (age %v): %v", age, n)
		}
	}
	tab.mutex.Lock()
	tab.stuff(seeds)
	tab.mutex.Unlock()

	// Finally, do a self lookup to fill up the buckets. 最后，进行自我查找以填满存储桶
	tab.lookup(tab.self.ID, false)
}

// closest returns the n nodes in the table that are closest to the
// given id. The caller must hold tab.mutex. 最接近返回表中最接近给定 id 的 n 个节点。 调用者必须持有 tab.mutex
func (tab *Table) closest(target common.Hash, nresults int) *nodesByDistance {
	// This is a very wasteful way to find the closest nodes but
	// obviously correct. I believe that tree-based buckets would make
	// this easier to implement efficiently. 这是找到最近节点的一种非常浪费的方法，但显然是正确的。 我相信基于树的存储桶将使这更容易有效地实现
	close := &nodesByDistance{target: target}
	for _, b := range tab.buckets {
		for _, n := range b.entries {
			close.push(n, nresults) // push 将给定节点添加到列表中，使总大小保持在 maxElems 以下。
		}
	}
	return close
}

func (tab *Table) len() (n int) {
	for _, b := range tab.buckets {
		n += len(b.entries)
	}
	return n
}

// bondall bonds with all given nodes concurrently and returns
// those nodes for which bonding has probably succeeded. bondall 同时与所有给定节点绑定，并返回绑定可能成功的节点
func (tab *Table) bondall(nodes []*Node) (result []*Node) {
	rc := make(chan *Node, len(nodes))
	for i := range nodes {
		go func(n *Node) {
			nn, _ := tab.bond(false, n.ID, n.addr(), uint16(n.TCP))
			rc <- nn
		}(nodes[i])
	}
	for _ = range nodes {
		if n := <-rc; n != nil {
			result = append(result, n)
		}
	}
	return result
}

// bond ensures the local node has a bond with the given remote node.
// It also attempts to insert the node into the table if bonding succeeds.
// The caller must not hold tab.mutex.
//
// A bond is must be established before sending findnode requests.
// Both sides must have completed a ping/pong exchange for a bond to
// exist. The total number of active bonding processes is limited in
// order to restrain network use.
//
// bond is meant to operate idempotently in that bonding with a remote
// node which still remembers a previously established bond will work.
// The remote node will simply not send a ping back, causing waitping
// to time out.
//
// If pinged is true, the remote node has just pinged us and one half
// of the process can be skipped.
// 绑定确保本地节点与给定的远程节点绑定。
// 如果绑定成功，它还会尝试将节点插入表中。
// 调用者不能持有 tab.mutex。
// 必须在发送 findnode 请求之前建立绑定。
// 双方必须完成 ping/pong 交换才能存在债券。 为了限制网络使用，活动绑定过程的总数受到限制。
//
// 绑定意味着幂等操作，因为与仍然记得先前建立的绑定的远程节点的绑定将起作用。
// 远程节点根本不会发回 ping，导致等待超时。
//
// 如果 pinged 为真，则远程节点刚刚 ping 我们，可以跳过一半的进程。
func (tab *Table) bond(pinged bool, id NodeID, addr *net.UDPAddr, tcpPort uint16) (*Node, error) {
	if id == tab.self.ID {
		return nil, errors.New("is self")
	}
	// Retrieve a previously known node and any recent findnode failures 检索以前已知的节点和任何最近的 findnode 故障
	node, fails := tab.db.node(id), 0
	if node != nil {
		fails = tab.db.findFails(id)
	}
	// If the node is unknown (non-bonded) or failed (remotely unknown), bond from scratch 如果节点未知（未绑定）或失败（远程未知），则从头开始绑定
	var result error
	age := time.Since(tab.db.lastPong(id))
	if node == nil || fails > 0 || age > nodeDBNodeExpiration {
		glog.V(logger.Detail).Infof("Bonding %x: known=%t, fails=%d age=%v", id[:8], node != nil, fails, age)

		tab.bondmu.Lock()
		w := tab.bonding[id]
		if w != nil {
			// Wait for an existing bonding process to complete. 等待现有的绑定过程完成
			tab.bondmu.Unlock()
			<-w.done
		} else {
			// Register a new bonding process. 注册一个新的绑定过程
			w = &bondproc{done: make(chan struct{})}
			tab.bonding[id] = w
			tab.bondmu.Unlock()
			// Do the ping/pong. The result goes into w. 打乒乓球。 结果进入 w
			tab.pingpong(w, pinged, id, addr, tcpPort)
			// Unregister the process after it's done. 完成后注销进程
			tab.bondmu.Lock()
			delete(tab.bonding, id)
			tab.bondmu.Unlock()
		}
		// Retrieve the bonding results 检索绑定结果
		result = w.err
		if result == nil {
			node = w.n
		}
	}
	if node != nil {
		// Add the node to the table even if the bonding ping/pong
		// fails. It will be relaced quickly if it continues to be
		// unresponsive. 即使绑定 ping/pong 失败，也将节点添加到表中。 如果它继续没有响应，它将很快被替换。
		tab.add(node)
		tab.db.updateFindFails(id, 0)
	}
	return node, result
}

func (tab *Table) pingpong(w *bondproc, pinged bool, id NodeID, addr *net.UDPAddr, tcpPort uint16) {
	// Request a bonding slot to limit network usage 请求绑定槽以限制网络使用
	<-tab.bondslots
	defer func() { tab.bondslots <- struct{}{} }()

	// Ping the remote side and wait for a pong.
	if w.err = tab.ping(id, addr); w.err != nil {
		close(w.done)
		return
	}
	if !pinged {
		// Give the remote node a chance to ping us before we start
		// sending findnode requests. If they still remember us,
		// waitping will simply time out. 在我们开始发送 findnode 请求之前，让远程节点有机会 ping 我们。 如果他们还记得我们，等待就会超时。
		tab.net.waitping(id)
	}

	glog.V(logger.Info).Infoln("tabel .... node")
	// Bonding succeeded, update the node database.
	w.n = NewNode(id, addr.IP, uint16(addr.Port), tcpPort)
	tab.db.updateNode(w.n)
	close(w.done)
}

// ping a remote endpoint and wait for a reply, also updating the node
// database accordingly. ping 远程端点并等待回复，同时相应地更新节点数据库
func (tab *Table) ping(id NodeID, addr *net.UDPAddr) error {
	tab.db.updateLastPing(id, time.Now())
	if err := tab.net.ping(id, addr); err != nil {
		return err
	}
	tab.db.updateLastPong(id, time.Now())

	// Start the background expiration goroutine after the first
	// successful communication. Subsequent calls have no effect if it
	// is already running. We do this here instead of somewhere else
	// so that the search for seed nodes also considers older nodes
	// that would otherwise be removed by the expiration.
	// 在第一次成功通信后启动后台过期 goroutine。 如果它已经在运行，则后续调用无效。
	// 我们在这里而不是在其他地方执行此操作，以便搜索种子节点时也会考虑到旧节点，否则这些节点会在到期时被删除。
	tab.db.ensureExpirer()
	return nil
}

// add attempts to add the given node its corresponding bucket. If the
// bucket has space available, adding the node succeeds immediately.
// Otherwise, the node is added if the least recently active node in
// the bucket does not respond to a ping packet.
//
// The caller must not hold tab.mutex.
// add 尝试将给定节点添加到其对应的存储桶。 如果
// 桶有空间，添加节点立即成功。
// 否则，如果节点中最近最少活动的节点，则添加该节点
// 存储桶不响应 ping 数据包。
//
// 调用者不能持有 tab.mutex
func (tab *Table) add(new *Node) {
	b := tab.buckets[logdist(tab.self.sha, new.sha)]
	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	if b.bump(new) {
		return
	}
	var oldest *Node
	if len(b.entries) == bucketSize {
		oldest = b.entries[bucketSize-1]
		if oldest.contested {
			// The node is already being replaced, don't attempt
			// to replace it.
			return
		}
		oldest.contested = true
		// Let go of the mutex so other goroutines can access
		// the table while we ping the least recently active node. 放开互斥锁，以便其他 goroutine 可以在我们 ping 最近最少活动的节点时访问该表
		tab.mutex.Unlock()
		err := tab.ping(oldest.ID, oldest.addr())
		tab.mutex.Lock()
		oldest.contested = false
		if err == nil {
			// The node responded, don't replace it.
			return
		}
	}
	added := b.replace(new, oldest)
	if added && tab.nodeAddedHook != nil {
		tab.nodeAddedHook(new)
	}
}

// stuff adds nodes the table to the end of their corresponding bucket
// if the bucket is not full. The caller must hold tab.mutex.
// stuff 将表中的节点添加到其相应存储桶的末尾
//  如果桶未满。 调用者必须持有 tab.mutex。
func (tab *Table) stuff(nodes []*Node) {
outer:
	for _, n := range nodes {
		if n.ID == tab.self.ID {
			continue // don't add self
		}
		bucket := tab.buckets[logdist(tab.self.sha, n.sha)]
		for i := range bucket.entries {
			if bucket.entries[i].ID == n.ID {
				continue outer // already in bucket
			}
		}
		if len(bucket.entries) < bucketSize {
			bucket.entries = append(bucket.entries, n)
			if tab.nodeAddedHook != nil {
				tab.nodeAddedHook(n)
			}
		}
	}
}

// delete removes an entry from the node table (used to evacuate
// failed/non-bonded discovery peers). delete 从节点表中删除一个条目（用于疏散失败/未绑定的发现对等方）
func (tab *Table) delete(node *Node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	bucket := tab.buckets[logdist(tab.self.sha, node.sha)]
	for i := range bucket.entries {
		if bucket.entries[i].ID == node.ID {
			bucket.entries = append(bucket.entries[:i], bucket.entries[i+1:]...)
			return
		}
	}
}

func (b *bucket) replace(n *Node, last *Node) bool {
	// Don't add if b already contains n.
	for i := range b.entries {
		if b.entries[i].ID == n.ID {
			return false
		}
	}
	// Replace last if it is still the last entry or just add n if b
	// isn't full. If is no longer the last entry, it has either been
	// replaced with someone else or became active.
	// 如果它仍然是最后一个条目，则替换 last ，如果 b 未满，则添加 n 。 如果不再是最后一个条目，则它已被其他人替换或变为活动状态
	if len(b.entries) == bucketSize && (last == nil || b.entries[bucketSize-1].ID != last.ID) {
		return false
	}
	if len(b.entries) < bucketSize {
		b.entries = append(b.entries, nil)
	}
	copy(b.entries[1:], b.entries)
	b.entries[0] = n
	return true
}

func (b *bucket) bump(n *Node) bool {
	for i := range b.entries {
		if b.entries[i].ID == n.ID {
			// move it to the front
			copy(b.entries[1:], b.entries[:i])
			b.entries[0] = n
			return true
		}
	}
	return false
}

// nodesByDistance is a list of nodes, ordered by
// distance to target.nodesByDistance 是一个节点列表，按到目标的距离排序。
type nodesByDistance struct {
	entries []*Node
	target  common.Hash
}

// push adds the given node to the list, keeping the total size below maxElems.
func (h *nodesByDistance) push(n *Node, maxElems int) {
	ix := sort.Search(len(h.entries), func(i int) bool {
		return distcmp(h.target, h.entries[i].sha, n.sha) > 0
	})
	if len(h.entries) < maxElems {
		h.entries = append(h.entries, n)
	}
	if ix == len(h.entries) {
		// farther away than all nodes we already have.
		// if there was room for it, the node is now the last element.
		// 比我们已经拥有的所有节点更远。
		// 如果有空间，节点现在是最后一个元素
	} else {
		// slide existing entries down to make room
		// this will overwrite the entry we just appended. 向下滑动现有条目以腾出空间，这将覆盖我们刚刚附加的条目
		copy(h.entries[ix+1:], h.entries[ix:])
		h.entries[ix] = n
	}
}

func (tab *Table) UpdateNode(id NodeID, nodetype uint64) error {
	glog.V(logger.Detail).Infof("table update node function ")
	oneNode := tab.GetNodeByID(id)
	aNode := &Node{
		IP:        oneNode.IP,
		UDP:       oneNode.UDP,
		TCP:       oneNode.TCP,
		ID:        oneNode.ID,
		sha:       oneNode.sha,
		contested: oneNode.contested,
		NodeType:  nodetype,
	}
	return tab.db.UpdateNodeByType(aNode)
}
func (tab *Table) GetNodeByID(id NodeID) *Node {
	return tab.db.GetNode(id)
}
