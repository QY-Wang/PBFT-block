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

// Package event implements an event multiplexer.
package event

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// Event is a time-tagged notification pushed to subscribers.  事件是推送给订阅者的带时间标签的通知
type Event struct {
	Time time.Time
	Data interface{}
}

// Subscription is implemented by event subscriptions.  订阅是通过事件订阅实现的。
type Subscription interface {
	// Chan returns a channel that carries events.
	// Implementations should return the same channel
	// for any subsequent calls to Chan.
	// Chan 返回一个携带事件的通道。
	// 实现应该为任何后续调用 Chan 返回相同的通道
	Chan() <-chan *Event

	// Unsubscribe stops delivery of events to a subscription.
	// The event channel is closed.
	// Unsubscribe can be called more than once.
	// 取消订阅会停止将事件传递给订阅。
	// 事件通道关闭。
	// 可以多次调用取消订阅。
	Unsubscribe()
}

// A TypeMux dispatches events to registered receivers. Receivers can be
// registered to handle events of certain type. Any operation
// called after mux is stopped will return ErrMuxClosed.
//
// The zero value is ready to use.
// TypeMux 将事件分派给注册的接收者。 可以注册接收器来处理特定类型的事件。 在 mux 停止后调用的任何操作都将返回 ErrMuxClosed。
// 零值可以使用了。
type TypeMux struct {
	mutex sync.RWMutex
	//	subm    map[reflect.Type][]*muxsub
	Subm    map[reflect.Type][]*muxsub // 变为public属性
	stopped bool
}

// ErrMuxClosed is returned when Posting on a closed TypeMux.  在关闭的 TypeMux 上发布时返回 ErrMuxClosed
var ErrMuxClosed = errors.New("event: mux closed")

// Subscribe creates a subscription for events of the given types. The
// subscription's channel is closed when it is unsubscribed
// or the mux is closed.  订阅为给定类型的事件创建订阅。 订阅的频道在取消订阅或 mux 关闭时关闭。
func (mux *TypeMux) Subscribe(types ...interface{}) Subscription {
	sub := newsub(mux)
	mux.mutex.Lock()
	defer mux.mutex.Unlock()
	if mux.stopped {
		// set the status to closed so that calling Unsubscribe after this
		// call will short curuit  将状态设置为关闭，以便在此调用后调用 Unsubscribe 将短路
		sub.closed = true
		close(sub.postC)
	} else {
		if mux.Subm == nil {
			mux.Subm = make(map[reflect.Type][]*muxsub)
		}
		for _, t := range types {
			rtyp := reflect.TypeOf(t)
			oldsubs := mux.Subm[rtyp]
			if find(oldsubs, sub) != -1 {
				panic(fmt.Sprintf("event: duplicate type %s in Subscribe", rtyp))
			}
			subs := make([]*muxsub, len(oldsubs)+1)
			copy(subs, oldsubs)
			subs[len(oldsubs)] = sub
			mux.Subm[rtyp] = subs
		}
	}
	return sub
}

// Post sends an event to all receivers registered for the given type.
// It returns ErrMuxClosed if the mux has been stopped.
func (mux *TypeMux) Post(ev interface{}) error {
	event := &Event{
		Time: time.Now(),
		Data: ev,
	}
	rtyp := reflect.TypeOf(ev)

	mux.mutex.RLock()
	if mux.stopped {
		mux.mutex.RUnlock()
		return ErrMuxClosed
	}
	subs := mux.Subm[rtyp]
	mux.mutex.RUnlock()
	for _, sub := range subs {
		sub.deliver(event)
	}
	return nil
}

// Stop closes a mux. The mux can no longer be used.
// Future Post calls will fail with ErrMuxClosed.
// Stop blocks until all current deliveries have finished.
func (mux *TypeMux) Stop() {
	mux.mutex.Lock()
	for _, subs := range mux.Subm {
		for _, sub := range subs {
			sub.closewait()
		}
	}
	mux.Subm = nil
	mux.stopped = true
	mux.mutex.Unlock()
}

func (mux *TypeMux) del(s *muxsub) {
	mux.mutex.Lock()
	for typ, subs := range mux.Subm {
		if pos := find(subs, s); pos >= 0 {
			if len(subs) == 1 {
				delete(mux.Subm, typ)
			} else {
				mux.Subm[typ] = posdelete(subs, pos)
			}
		}
	}
	s.mux.mutex.Unlock()
}

func find(slice []*muxsub, item *muxsub) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

func posdelete(slice []*muxsub, pos int) []*muxsub {
	news := make([]*muxsub, len(slice)-1)
	copy(news[:pos], slice[:pos])
	copy(news[pos:], slice[pos+1:])
	return news
}

type muxsub struct {
	mux     *TypeMux
	created time.Time
	closeMu sync.Mutex
	closing chan struct{}
	closed  bool

	// these two are the same channel. they are stored separately so
	// postC can be set to nil without affecting the return value of
	// Chan.  这两个是同一个频道。 它们是分开存储的，所以 postC 可以设置为 nil 而不会影响 Chan 的返回值
	postMu sync.RWMutex
	readC  <-chan *Event //接受通道
	postC  chan<- *Event //发送通道
}

func newsub(mux *TypeMux) *muxsub {
	c := make(chan *Event) //双向通道
	return &muxsub{
		mux:     mux,
		created: time.Now(),
		readC:   c,
		postC:   c,
		closing: make(chan struct{}),
	}
}

func (s *muxsub) Chan() <-chan *Event {
	return s.readC
}

func (s *muxsub) Unsubscribe() {
	s.mux.del(s)
	s.closewait()
}

func (s *muxsub) closewait() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return
	}
	close(s.closing)
	s.closed = true

	s.postMu.Lock()
	close(s.postC)
	s.postC = nil
	s.postMu.Unlock()
}

func (s *muxsub) deliver(event *Event) {
	// Short circuit delivery if stale event  如果过时事件发生短路传递
	if s.created.After(event.Time) {
		return
	}
	// Otherwise deliver the event
	s.postMu.RLock()
	defer s.postMu.RUnlock()

	select {
	case s.postC <- event:
	case <-s.closing:
	}
}
