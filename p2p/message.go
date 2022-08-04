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
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DWBC-ConPeer/logger"
	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/rlp"
	//	"github.com/DWBC-ConPeer/core"
)

// Msg defines the structure of a p2p message.
//
// Note that a Msg can only be sent once since the Payload reader is
// consumed during sending. It is not possible to create a Msg and
// send it any number of times. If you want to reuse an encoded
// structure, encode the payload into a byte array and create a
// separate Msg with a bytes.Reader as Payload for each send.
// Msg 定义了 p2p 消息的结构。
//
// 注意，一个Msg只能发送一次，因为发送过程中会消耗Payload reader。不可能创建一个Msg并发送任意次数。
//
// 如果你想复用一个编码结构，把payload编码成一个字节数组并为每个发送创建一个单独的带有 bytes.Reader 作为 Payload 的 Msg。
type Msg struct {
	Code       uint64
	Size       uint32 // size of the paylod
	Payload    io.Reader
	ReceivedAt time.Time
}

// Decode parses the RLP content of a message into
// the given value, which must be a pointer.
//
// For the decoding rules, please see package rlp.
// Decode 将消息的 RLP 内容解析为给定的值，该值必须是一个指针。
// 解码规则见package rlp
func (msg Msg) Decode(val interface{}) error {
	s := rlp.NewStream(msg.Payload, uint64(msg.Size))
	if err := s.Decode(val); err != nil {
		return newPeerError(errInvalidMsg, "(code %x) (size %d) %v", msg.Code, msg.Size, err)
	}
	return nil
}

func (msg Msg) String() string {
	return fmt.Sprintf("msg #%v (%v bytes)", msg.Code, msg.Size)
}

// Discard reads any remaining payload data into a black hole.
// Discard 将任何剩余的有效载荷数据读入黑洞
func (msg Msg) Discard() error {
	_, err := io.Copy(ioutil.Discard, msg.Payload)
	return err
}

type MsgReader interface {
	ReadMsg() (Msg, error)
}

type MsgWriter interface {
	// WriteMsg sends a message. It will block until the message's
	// Payload has been consumed by the other end.
	//
	// Note that messages can be sent only once because their
	// payload reader is drained.
	//WriteMsg 发送一条消息。它会阻塞直到消息的
	// 有效负载已被另一端消耗。
	// 请注意，消息只能发送一次，因为它们的
	// 有效负载阅读器已耗尽。
	WriteMsg(Msg) error
}

// MsgReadWriter provides reading and writing of encoded messages.
// Implementations should ensure that ReadMsg and WriteMsg can be
// called simultaneously from multiple goroutines.
// MsgReadWriter 提供对编码消息的读取和写入。
// 实现应该确保可以从多个 goroutine 同时调用 ReadMsg 和 WriteMsg。
type MsgReadWriter interface {
	MsgReader
	MsgWriter
}

// Send writes an RLP-encoded message with the given code.
// data should encode as an RLP list.
// Send 使用给定的代码写入 RLP 编码的消息。数据应编码为 RLP 列表
func Send(w MsgWriter, msgcode uint64, data interface{}) error {
	//	glog.V(logger.Info).Infof("p2p.send data=%d ", data)
	size, r, err := rlp.EncodeToReader(data)
	if err != nil {
		glog.V(logger.Info).Infof("EncodeToReader err=%d ", err)
		return err
	}

	return w.WriteMsg(Msg{Code: msgcode, Size: uint32(size), Payload: r})
}

// SendItems writes an RLP with the given code and data elements.
// For a call such as:
//
//    SendItems(w, code, e1, e2, e3)
//
// the message payload will be an RLP list containing the items:
//    [e1, e2, e3]
// SendItems 使用给定的代码和数据元素编写 RLP。
// 对于诸如以下的呼叫：
// SendItems (w, code, e1, e2, e3)
// 消息有效负载将是一个包含以下项目的 RLP 列表：
// [e1，e2，e3]
func SendItems(w MsgWriter, msgcode uint64, elems ...interface{}) error {
	return Send(w, msgcode, elems)
}

// netWrapper wraps a MsgReadWriter with locks around
// ReadMsg/WriteMsg and applies read/write deadlines.
// netWrapper 用锁包装了一个 MsgReadWriter
// ReadMsg / WriteMsg 并应用读/写截止日期。
type netWrapper struct {
	rmu, wmu sync.Mutex

	rtimeout, wtimeout time.Duration
	conn               net.Conn
	wrapped            MsgReadWriter
}

func (rw *netWrapper) ReadMsg() (Msg, error) {
	rw.rmu.Lock()
	defer rw.rmu.Unlock()
	rw.conn.SetReadDeadline(time.Now().Add(rw.rtimeout))
	return rw.wrapped.ReadMsg()
}

func (rw *netWrapper) WriteMsg(msg Msg) error {
	glog.V(logger.Info).Infoln("is this here message WriteMsg?")
	rw.wmu.Lock()
	defer rw.wmu.Unlock()
	rw.conn.SetWriteDeadline(time.Now().Add(rw.wtimeout))

	return rw.wrapped.WriteMsg(msg)
}

// eofSignal wraps a reader with eof signaling. the eof channel is
// closed when the wrapped reader returns an error or when count bytes
// have been read.eofSignal 用 eof 信号封装阅读器。eof 通道是
//当包装的阅读器返回错误或已读取计数字节时关闭。
type eofSignal struct {
	wrapped io.Reader
	count   uint32 // number of bytes left  剩余字节数
	eof     chan<- struct{}
}

// note: when using eofSignal to detect whether a message payload
// has been read, Read might not be called for zero sized messages. 注意：当使用 eofSignal 检测消息负载是否已被读取时，可能不会为零大小的消息调用 Read。
func (r *eofSignal) Read(buf []byte) (int, error) {
	if r.count == 0 {
		if r.eof != nil {
			r.eof <- struct{}{}
			r.eof = nil
		}
		return 0, io.EOF
	}

	max := len(buf)
	if int(r.count) < len(buf) {
		max = int(r.count)
	}
	n, err := r.wrapped.Read(buf[:max])
	r.count -= uint32(n)
	if (err != nil || r.count == 0) && r.eof != nil {
		r.eof <- struct{}{} // tell Peer that msg has been consumed 告诉 Peer msg 已被消耗
		r.eof = nil
	}
	return n, err
}

// MsgPipe creates a message pipe. Reads on one end are matched
// with writes on the other. The pipe is full-duplex, both ends
// implement MsgReadWriter. MsgPipe 创建一个消息管道，一端读匹配另一端写，管道是全双工的，两端都实现MsgReadWriter。
func MsgPipe() (*MsgPipeRW, *MsgPipeRW) {
	var (
		c1, c2  = make(chan Msg), make(chan Msg)
		closing = make(chan struct{})
		closed  = new(int32)
		rw1     = &MsgPipeRW{c1, c2, closing, closed}
		rw2     = &MsgPipeRW{c2, c1, closing, closed}
	)
	return rw1, rw2
}

// ErrPipeClosed is returned from pipe operations after the
// pipe has been closed. ErrPipeClosed 在管道关闭后从管道操作中返回。
var ErrPipeClosed = errors.New("p2p: read or write on closed message pipe")

// MsgPipeRW is an endpoint of a MsgReadWriter pipe.  MsgPipeRW 是 MsgReadWriter 管道的端点
type MsgPipeRW struct {
	w       chan<- Msg
	r       <-chan Msg
	closing chan struct{}
	closed  *int32
}

// WriteMsg sends a messsage on the pipe.
// It blocks until the receiver has consumed the message payload.
// WriteMsg 在管道上发送消息。
// 它会一直阻塞，直到接收者使用完消息有效负载。
func (p *MsgPipeRW) WriteMsg(msg Msg) error {
	if atomic.LoadInt32(p.closed) == 0 {
		consumed := make(chan struct{}, 1)
		msg.Payload = &eofSignal{msg.Payload, msg.Size, consumed}
		select {
		case p.w <- msg:
			if msg.Size > 0 {
				// wait for payload read or discard
				select {
				case <-consumed:
				case <-p.closing:
				}
			}
			return nil
		case <-p.closing:
		}
	}
	return ErrPipeClosed
}

// ReadMsg returns a message sent on the other end of the pipe. ReadMsg 返回在管道另一端发送的消息。
func (p *MsgPipeRW) ReadMsg() (Msg, error) {
	if atomic.LoadInt32(p.closed) == 0 {
		select {
		case msg := <-p.r:
			return msg, nil
		case <-p.closing:
		}
	}
	return Msg{}, ErrPipeClosed
}

// Close unblocks any pending ReadMsg and WriteMsg calls on both ends
// of the pipe. They will return ErrPipeClosed. Close also
// interrupts any reads from a message payload.
// Close 取消阻塞管道两端的任何挂起的 ReadMsg 和 WriteMsg 调用。它们将返回 ErrPipeClosed。Close 还会中断来自消息负载的任何读取。
func (p *MsgPipeRW) Close() error {
	if atomic.AddInt32(p.closed, 1) != 1 {
		// someone else is already closing
		atomic.StoreInt32(p.closed, 1) // avoid overflow 避免溢出
		return nil
	}
	close(p.closing)
	return nil
}

// ExpectMsg reads a message from r and verifies that its
// code and encoded RLP content match the provided values.
// If content is nil, the payload is discarded and not verified.
// ExpectMsg 从 r 读取消息并验证其代码和编码的 RLP 内容是否与提供的值匹配。如果内容为 nil，则丢弃有效负载并且不进行验证。
func ExpectMsg(r MsgReader, code uint64, content interface{}) error {
	msg, err := r.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != code {
		return fmt.Errorf("message code mismatch: got %d, expected %d", msg.Code, code)
	}
	if content == nil {
		return msg.Discard()
	} else {
		contentEnc, err := rlp.EncodeToBytes(content)
		if err != nil {
			panic("content encode error: " + err.Error())
		}
		if int(msg.Size) != len(contentEnc) {
			return fmt.Errorf("message size mismatch: got %d, want %d", msg.Size, len(contentEnc))
		}
		actualContent, err := ioutil.ReadAll(msg.Payload)
		if err != nil {
			return err
		}
		if !bytes.Equal(actualContent, contentEnc) {
			return fmt.Errorf("message payload mismatch:\ngot:  %x\nwant: %x", actualContent, contentEnc)
		}
	}
	return nil
}
