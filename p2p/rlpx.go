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
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	crypto "github.com/DWBC-ConPeer/crypto-gm"
	"github.com/DWBC-ConPeer/crypto-gm/sm2"
	"github.com/DWBC-ConPeer/crypto-gm/sm3"
	"hash"
	"io"
	mrand "math/rand"
	"net"
	"sync"
	"time"

	//	"github.com/DWBC-ConPeer/logger"
	//	"github.com/DWBC-ConPeer/logger/glog"
	"github.com/DWBC-ConPeer/p2p/discover"
	"github.com/DWBC-ConPeer/rlp"
)

const (
	maxUint24 = ^uint32(0) >> 8

	sskLen = 16  // ecies.MaxSharedKeyLength(pubKey) / 2
	sigLen = 140 // elliptic S256 65 国标签名不是定长，暂时取最大值，后面根据实际情况截取
	pubLen = 64  // 512 bit pubkey in uncompressed representation without format byte
	shaLen = 32  // hash length (for nonce etc)

	authMsgLen  = sigLen + shaLen + pubLen + shaLen + 1
	authRespLen = pubLen + shaLen + 1

	eciesOverhead = 65 /* pubkey */ + 16 /* IV */ + 32 /* MAC */

	encAuthMsgLen  = authMsgLen + eciesOverhead  // size of encrypted pre-EIP-8 initiator handshake
	encAuthRespLen = authRespLen + eciesOverhead // size of encrypted pre-EIP-8 handshake reply

	// total timeout for encryption handshake and protocol
	// handshake in both directions.
	handshakeTimeout = 5 * time.Second

	// This is the timeout for sending the disconnect reason.
	// This is shorter than the usual timeout because we don't want
	// to wait if the connection is known to be bad anyway.
	discWriteTimeout = 1 * time.Second
)

// rlpx is the transport protocol used by actual (non-test) connections.
// It wraps the frame encoder with locks and read/write deadlines.
// rlpx 是实际（非测试）连接使用的传输协议。
// 它用锁和读/写截止日期包装帧编码器。
type rlpx struct {
	fd net.Conn

	rmu, wmu sync.Mutex
	rw       *rlpxFrameRW
}

func newRLPX(fd net.Conn) transport {
	fd.SetDeadline(time.Now().Add(handshakeTimeout))
	return &rlpx{fd: fd}
}

func (t *rlpx) ReadMsg() (Msg, error) {
	t.rmu.Lock()
	defer t.rmu.Unlock()
	t.fd.SetReadDeadline(time.Now().Add(frameReadTimeout))
	return t.rw.ReadMsg()
}

func (t *rlpx) WriteMsg(msg Msg) error {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	t.fd.SetWriteDeadline(time.Now().Add(frameWriteTimeout))
	return t.rw.WriteMsg(msg)
}

func (t *rlpx) close(err error) {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	// Tell the remote end why we're disconnecting if possible. 如果可能的话，告诉远端我们为什么要断开连接。
	if t.rw != nil {
		if r, ok := err.(DiscReason); ok && r != DiscNetworkError {
			t.fd.SetWriteDeadline(time.Now().Add(discWriteTimeout))
			SendItems(t.rw, discMsg, r)
		}
	}
	t.fd.Close()
}

// doEncHandshake runs the protocol handshake using authenticated
// messages. the protocol handshake is the first authenticated message
// and also verifies whether the encryption handshake 'worked' and the
// remote side actually provided the right public key.

// doEncHandshake 使用经过身份验证的消息运行协议握手。协议握手是第一个经过身份验证的消息，并且还验证加密握手是否“有效”以及
// 远程端实际上提供了正确的公钥。
func (t *rlpx) doProtoHandshake(our *protoHandshake) (their *protoHandshake, err error) {
	// Writing our handshake happens concurrently, we prefer
	// returning the handshake read error. If the remote side
	// disconnects us early with a valid reason, we should return it
	// as the error so it can be tracked elsewhere.
	// 写握手是同时发生的，我们更喜欢返回握手读取错误。如果远程端有正当理由提前断开我们的连接，我们应该将其作为错误返回，以便在其他地方进行跟踪。
	werr := make(chan error, 1)
	go func() { werr <- Send(t.rw, handshakeMsg, our) }()
	if their, err = readProtocolHandshake(t.rw, our); err != nil {
		<-werr // make sure the write terminates too 确保写入也终止
		return nil, err
	}
	if err := <-werr; err != nil {
		return nil, fmt.Errorf("write error: %v", err)
	}
	return their, nil
}

func readProtocolHandshake(rw MsgReader, our *protoHandshake) (*protoHandshake, error) {
	msg, err := rw.ReadMsg()
	if err != nil {
		return nil, err
	}
	if msg.Size > baseProtocolMaxMsgSize {
		return nil, fmt.Errorf("message too big")
	}
	if msg.Code == discMsg {
		// 根据规范，在协议握手有效之前断开连接，如果 posthanshake 检查失败，我们会自行发送。
		// 但是，我们不能直接返回原因，因为否则会回显。改为将其包装在字符串中。
		// Disconnect before protocol handshake is valid according to the
		// spec and we send it ourself if the posthanshake checks fail.
		// We can't return the reason directly, though, because it is echoed
		// back otherwise. Wrap it in a string instead.
		var reason [1]DiscReason
		rlp.Decode(msg.Payload, &reason)
		return nil, reason[0]
	}
	if msg.Code != handshakeMsg {
		return nil, fmt.Errorf("expected handshake, got %x", msg.Code)
	}
	var hs protoHandshake
	if err := msg.Decode(&hs); err != nil {
		return nil, err
	}
	if (hs.ID == discover.NodeID{}) {
		return nil, DiscInvalidIdentity
	}
	return &hs, nil
}

//prv 节点已知静态私钥
func (t *rlpx) doEncHandshake(prv *sm2.PrivateKey, dial *discover.Node) (discover.NodeID, error) {
	var (
		sec secrets
		err error
	)
	if dial == nil {
		sec, err = receiverEncHandshake(t.fd, prv, nil)
	} else {
		sec, err = initiatorEncHandshake(t.fd, prv, dial.ID, nil) //发起者EncHandshake 在 conn 上协商会话令牌。 它应该在连接的拨号端调用。 prv 是本地客户端的私钥
	}
	if err != nil {
		return discover.NodeID{}, err
	}
	t.wmu.Lock()
	t.rw = newRLPXFrameRW(t.fd, sec)
	t.wmu.Unlock()
	return sec.RemoteID, nil
}

// encHandshake contains the state of the encryption handshake. encHandshake 包含加密握手的状态
type encHandshake struct {
	initiator bool
	remoteID  discover.NodeID

	remotePub            *sm2.PublicKey  // remote-pubk
	initNonce, respNonce []byte          // nonce
	randomPrivKey        *sm2.PrivateKey // ecdhe-random ecdhe-随机
	remoteRandomPub      *sm2.PublicKey  // ecdhe-random-pubk
}

// secrets represents the connection secrets
// which are negotiated during the encryption handshake. secrets 表示在加密握手期间协商的连接秘密。
type secrets struct {
	RemoteID              discover.NodeID
	AES, MAC              []byte
	EgressMAC, IngressMAC hash.Hash // 进出口MAC
	Token                 []byte
}

// RLPx v4 handshake auth (defined in EIP-8). RLPx v4 握手认证（在 EIP-8 中定义）。
type authMsgV4 struct {
	gotPlain bool // whether read packet had plain format. 读取数据包是否为纯格式。

	Signature       [sigLen]byte
	InitiatorPubkey [pubLen]byte
	Nonce           [shaLen]byte
	Version         uint
	SigSize         uint
	// Ignore additional fields (forward-compatibility)
	Rest []rlp.RawValue `rlp:"tail"`
}

// RLPx v4 handshake response (defined in EIP-8). RLPx v4 握手响应（在 EIP-8 中定义）。
type authRespV4 struct {
	RandomPubkey [pubLen]byte
	Nonce        [shaLen]byte
	Version      uint

	// Ignore additional fields (forward-compatibility)
	Rest []rlp.RawValue `rlp:"tail"`
}

// secrets is called after the handshake is completed.
// It extracts the connection secrets from the handshake values.
// 握手完成后调用secrets。
//  它从握手值中提取连接秘密。
func (h *encHandshake) secrets(auth, authResp []byte) (secrets, error) {
	ecdheSecret, err := h.randomPrivKey.GenerateShared(h.remoteRandomPub, sskLen, sskLen)
	if err != nil {
		return secrets{}, err
	}

	// derive base secrets from ephemeral key agreement 从临时密钥协议中导出基本秘密
	sharedSecret := crypto.Keccak256(ecdheSecret, crypto.Keccak256(h.respNonce, h.initNonce))
	aesSecret := crypto.Keccak256(ecdheSecret, sharedSecret)
	s := secrets{
		RemoteID: h.remoteID,
		AES:      aesSecret,
		MAC:      crypto.Keccak256(ecdheSecret, aesSecret),
	}

	// setup sha3 instances for the MACs
	mac1 := sm3.NewKeccak256()
	mac1.Write(xor(s.MAC, h.respNonce))
	mac1.Write(auth)
	mac2 := sm3.NewKeccak256()
	mac2.Write(xor(s.MAC, h.initNonce)) //或操作
	mac2.Write(authResp)
	if h.initiator {
		s.EgressMAC, s.IngressMAC = mac1, mac2
	} else {
		s.EgressMAC, s.IngressMAC = mac2, mac1
	}

	return s, nil
}

// staticSharedSecret returns the static shared secret, the result
// of key agreement between the local and remote static node key. staticSharedSecret 返回静态共享密钥，本地和远程静态节点密钥的密钥协商结果
func (h *encHandshake) staticSharedSecret(prv *sm2.PrivateKey) ([]byte, error) {
	return prv.GenerateShared(h.remotePub, sskLen, sskLen)
}

// initiatorEncHandshake negotiates a session token on conn.
// it should be called on the dialing side of the connection.
//
// prv is the local client's private key.
//prv本节点静态私钥 token入参取值为nil
func initiatorEncHandshake(conn io.ReadWriter, prv *sm2.PrivateKey, remoteID discover.NodeID, token []byte) (s secrets, err error) {
	// 初始化握手对象
	h := &encHandshake{initiator: true, remoteID: remoteID}
	// 生成验证信息
	authMsg, err := h.makeAuthMsg(prv, token)
	if err != nil {
		return s, err
	}
	// 封包,将验证信息和握手进行rlp编码并拼接前缀信息
	authPacket, err := sealEIP8(authMsg, h)
	if err != nil {
		return s, err
	}
	// 通过conn发送消息
	if _, err = conn.Write(authPacket); err != nil {
		return s, err
	}

	// 处理接收的信息,得到响应包
	authRespMsg := new(authRespV4)
	authRespPacket, err := readHandshakeMsg(authRespMsg, encAuthRespLen, prv, conn)
	if err != nil {
		return s, err
	}
	// 填充响应的respNonce(对方随机数,生成共享私钥用)和remoteRandomPub(对方的随机公钥)
	if err := h.handleAuthResp(authRespMsg); err != nil {
		return s, err
	}
	// 将请求包和响应包封装成共享秘密(secrets)
	return h.secrets(authPacket, authRespPacket)
}

// makeAuthMsg creates the initiator handshake message.
//prv 本节点静态私钥// token nil
func (h *encHandshake) makeAuthMsg(prv *sm2.PrivateKey, token []byte) (*authMsgV4, error) {
	// 得到接收方的公钥地址
	rpub, err := h.remoteID.Pubkey() //对方节点静态公钥
	if err != nil {
		return nil, fmt.Errorf("bad remoteID: %v", err)
	}
	h.remotePub = rpub
	// Generate random initiator nonce.生成自己方的随机数
	h.initNonce = make([]byte, shaLen)
	if _, err := rand.Read(h.initNonce); err != nil {
		return nil, err
	}
	// Generate random keypair to for ECDH.// 生成随机的一组公私钥对
	h.randomPrivKey, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	// 生成静态共享秘密token(用己方私钥和对方公钥进行有限域乘法)
	// Sign known message: static-shared-secret ^ nonce
	token, err = h.staticSharedSecret(prv)
	if err != nil {
		return nil, err
	}
	// 和己方随机数异或后用随机生成的私钥签名
	signed := xor(token, h.initNonce)
	signature, err := crypto.Sign(signed, h.randomPrivKey)
	if err != nil {
		return nil, err
	}

	msg := new(authMsgV4)
	copy(msg.Signature[:], signature)
	copy(msg.InitiatorPubkey[:], crypto.FromECDSAPub(&prv.PublicKey)[1:])
	copy(msg.Nonce[:], h.initNonce)
	msg.SigSize = uint(len(signature))
	msg.Version = 4
	return msg, nil
}

func (h *encHandshake) handleAuthResp(msg *authRespV4) (err error) {
	h.respNonce = msg.Nonce[:]
	h.remoteRandomPub, err = importPublicKey(msg.RandomPubkey[:])
	return err
}

// receiverEncHandshake negotiates a session token on conn.
// it should be called on the listening side of the connection.
//
// prv is the local client's private key.
// token is the token from a previous session with this node.
// receiverEncHandshake 在 conn 上协商会话令牌。它应该在连接的侦听端调用。prv 是本地客户端的私钥。 token 是与此节点的先前会话中的令牌
func receiverEncHandshake(conn io.ReadWriter, prv *sm2.PrivateKey, token []byte) (s secrets, err error) {
	authMsg := new(authMsgV4)
	authPacket, err := readHandshakeMsg(authMsg, encAuthMsgLen, prv, conn)
	if err != nil {
		return s, err
	}
	h := new(encHandshake)
	if err := h.handleAuthMsg(authMsg, prv); err != nil {
		return s, err
	}

	authRespMsg, err := h.makeAuthResp()
	if err != nil {
		return s, err
	}
	var authRespPacket []byte
	if authMsg.gotPlain {
		authRespPacket, err = authRespMsg.sealPlain(h)
	} else {
		authRespPacket, err = sealEIP8(authRespMsg, h)
	}
	if err != nil {
		return s, err
	}
	if _, err = conn.Write(authRespPacket); err != nil {
		return s, err
	}
	return h.secrets(authPacket, authRespPacket)
}

func (h *encHandshake) handleAuthMsg(msg *authMsgV4, prv *sm2.PrivateKey) error {
	// Import the remote identity.
	h.initNonce = msg.Nonce[:]
	h.remoteID = msg.InitiatorPubkey
	rpub, err := h.remoteID.Pubkey()
	if err != nil {
		return fmt.Errorf("bad remoteID: %#v", err)
	}
	h.remotePub = rpub

	// Generate random keypair for ECDH.
	// If a private key is already set, use it instead of generating one (for testing).
	// 为 ECDH 生成随机密钥对。
	// 如果已经设置了私钥，请使用它而不是生成一个（用于测试）
	if h.randomPrivKey == nil {
		h.randomPrivKey, err = crypto.GenerateKey()
		if err != nil {
			return err
		}
	}

	// Check the signature.
	token, err := h.staticSharedSecret(prv)
	if err != nil {
		return err
	}
	signedMsg := xor(token, h.initNonce)
	sig := msg.Signature[:]
	if int(msg.SigSize) != len(sig) {
		sig = sig[:msg.SigSize]
	}
	remoteRandomPub, err := crypto.Ecrecover(signedMsg, sig) //推测出远端随机公钥
	if err != nil {
		return err
	}
	h.remoteRandomPub, err = importPublicKey(remoteRandomPub)
	return err
}

func (h *encHandshake) makeAuthResp() (msg *authRespV4, err error) {
	// Generate random nonce.
	h.respNonce = make([]byte, shaLen)
	if _, err = rand.Read(h.respNonce); err != nil {
		return nil, err
	}

	msg = new(authRespV4)
	copy(msg.Nonce[:], h.respNonce)
	copy(msg.RandomPubkey[:], exportPubkey(&h.randomPrivKey.PublicKey))
	msg.Version = 4
	return msg, nil
}

func (msg *authMsgV4) sealPlain(h *encHandshake) ([]byte, error) {
	buf := make([]byte, authMsgLen)
	n := copy(buf, msg.Signature[:])
	n += copy(buf[n:], crypto.Keccak256(exportPubkey(&h.randomPrivKey.PublicKey)))
	n += copy(buf[n:], msg.InitiatorPubkey[:])
	n += copy(buf[n:], msg.Nonce[:])
	buf[n] = 0 // token-flag
	return crypto.Encrypt(h.remotePub, buf)
}

func (msg *authMsgV4) decodePlain(input []byte) {
	n := copy(msg.Signature[:], input)
	n += shaLen // skip sha3(initiator-ephemeral-pubk)
	n += copy(msg.InitiatorPubkey[:], input[n:])
	n += copy(msg.Nonce[:], input[n:])
	msg.Version = 4
	msg.gotPlain = true
}

func (msg *authRespV4) sealPlain(hs *encHandshake) ([]byte, error) {
	buf := make([]byte, authRespLen)
	n := copy(buf, msg.RandomPubkey[:])
	n += copy(buf[n:], msg.Nonce[:])
	return crypto.Encrypt(hs.remotePub, buf)
}

func (msg *authRespV4) decodePlain(input []byte) {
	n := copy(msg.RandomPubkey[:], input)
	n += copy(msg.Nonce[:], input[n:])
	msg.Version = 4
}

var padSpace = make([]byte, 300)

func sealEIP8(msg interface{}, h *encHandshake) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := rlp.Encode(buf, msg); err != nil {
		return nil, err
	}
	// pad with random amount of data. the amount needs to be at least 100 bytes to make
	// the message distinguishable from pre-EIP-8 handshakes.
	pad := padSpace[:mrand.Intn(len(padSpace)-100)+100]
	buf.Write(pad)
	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(buf.Len()+eciesOverhead))

	//enc, err := ecies.Encrypt(rand.Reader, h.remotePub, buf.Bytes(), nil, prefix) //FIXME 此处做了简化，没有对prefix进行加密处理
	enc, err := sm2.Encrypt(h.remotePub, buf.Bytes())
	return append(prefix, enc...), err
}

type plainDecoder interface {
	decodePlain([]byte)
}

func readHandshakeMsg(msg plainDecoder, plainSize int, prv *sm2.PrivateKey, r io.Reader) ([]byte, error) {
	buf := make([]byte, plainSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return buf, err
	}
	// Attempt decoding pre-EIP-8 "plain" format.
	if dec, err := prv.Decrypt(buf); err == nil {
		msg.decodePlain(dec)
		return buf, nil
	}
	// Could be EIP-8 format, try that.
	prefix := buf[:2]
	size := binary.BigEndian.Uint16(prefix)
	if size < uint16(plainSize) {
		return buf, fmt.Errorf("size underflow, need at least %d bytes", plainSize)
	}
	buf = append(buf, make([]byte, size-16-uint16(plainSize)+2)...) //FIXME 改为国密后，需要再减16适配才能
	if _, err := io.ReadFull(r, buf[plainSize:]); err != nil {
		return buf, err
	}
	dec, err := prv.Decrypt(buf[2:])
	if err != nil {
		return buf, err
	}
	// Can't use rlp.DecodeBytes here because it rejects
	// trailing data (forward-compatibility). 不能在此处使用 rlp.DecodeBytes，因为它拒绝尾随数据（前向兼容性）
	s := rlp.NewStream(bytes.NewReader(dec), 0)
	return buf, s.Decode(msg)
}

// importPublicKey unmarshals 512 bit public keys.
func importPublicKey(pubKey []byte) (*sm2.PublicKey, error) {
	var pubKey65 []byte
	switch len(pubKey) {
	case 64:
		// add 'uncompressed key' flag
		pubKey65 = append([]byte{0x04}, pubKey...)
	case 65:
		pubKey65 = pubKey
	default:
		return nil, fmt.Errorf("invalid public key length %v (expect 64/65)", len(pubKey))
	}
	pub := crypto.ToECDSAPub(pubKey65)
	if pub.X == nil {
		return nil, fmt.Errorf("invalid public key")
	}
	return pub, nil
}

func exportPubkey(pub *sm2.PublicKey) []byte {
	if pub == nil {
		panic("nil pubkey")
	}
	return crypto.FromECDSAPub(pub)[1:]
}

func xor(one, other []byte) (xor []byte) {
	xor = make([]byte, len(one))
	for i := 0; i < len(one); i++ {
		xor[i] = one[i] ^ other[i]
	}
	return xor
}

var (
	// this is used in place of actual frame header data.
	// TODO: replace this when Msg contains the protocol type code.
	zeroHeader = []byte{0xC2, 0x80, 0x80}
	// sixteen zero bytes
	zero16 = make([]byte, 16)
)

// rlpxFrameRW implements a simplified version of RLPx framing.
// chunked messages are not supported and all headers are equal to
// zeroHeader.
//
// rlpxFrameRW is not safe for concurrent use from multiple goroutines.
// rlpxFrameRW 实现了 RLPx 框架的简化版本。
// 不支持分块消息，并且所有标头都等于 zeroHeader。
// rlpxFrameRW 对于多个 goroutine 的并发使用是不安全的。
type rlpxFrameRW struct {
	conn io.ReadWriter
	enc  cipher.Stream
	dec  cipher.Stream

	macCipher  cipher.Block
	egressMAC  hash.Hash
	ingressMAC hash.Hash
}

func newRLPXFrameRW(conn io.ReadWriter, s secrets) *rlpxFrameRW {
	macc, err := aes.NewCipher(s.MAC)
	if err != nil {
		panic("invalid MAC secret: " + err.Error())
	}
	encc, err := aes.NewCipher(s.AES)
	if err != nil {
		panic("invalid AES secret: " + err.Error())
	}
	// we use an all-zeroes IV for AES because the key used
	// for encryption is ephemeral. 我们对 AES 使用全零 IV，因为用于加密的密钥是临时的
	iv := make([]byte, encc.BlockSize())
	return &rlpxFrameRW{
		conn:       conn,
		enc:        cipher.NewCTR(encc, iv),
		dec:        cipher.NewCTR(encc, iv),
		macCipher:  macc,
		egressMAC:  s.EgressMAC,
		ingressMAC: s.IngressMAC,
	}
}

func (rw *rlpxFrameRW) WriteMsg(msg Msg) error {
	ptype, _ := rlp.EncodeToBytes(msg.Code)

	// write header
	headbuf := make([]byte, 32)
	fsize := uint32(len(ptype)) + msg.Size
	if fsize > maxUint24 {
		return errors.New("message size overflows uint24")
	}
	putInt24(fsize, headbuf) // TODO: check overflow
	copy(headbuf[3:], zeroHeader)
	rw.enc.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now encrypted

	// write header MAC
	copy(headbuf[16:], updateMAC(rw.egressMAC, rw.macCipher, headbuf[:16]))
	if _, err := rw.conn.Write(headbuf); err != nil {
		return err
	}

	// write encrypted frame, updating the egress MAC hash with
	// the data written to conn.
	tee := cipher.StreamWriter{S: rw.enc, W: io.MultiWriter(rw.conn, rw.egressMAC)}
	if _, err := tee.Write(ptype); err != nil {
		return err
	}
	if _, err := io.Copy(tee, msg.Payload); err != nil {
		return err
	}
	if padding := fsize % 16; padding > 0 {
		if _, err := tee.Write(zero16[:16-padding]); err != nil {
			return err
		}
	}

	// write frame MAC. egress MAC hash is up to date because
	// frame content was written to it as well.
	fmacseed := rw.egressMAC.Sum(nil)
	mac := updateMAC(rw.egressMAC, rw.macCipher, fmacseed)
	_, err := rw.conn.Write(mac)
	//	glog.V(logger.Info).Infof("ask ask is bu is this mac=%d", mac)
	return err
}

func (rw *rlpxFrameRW) ReadMsg() (msg Msg, err error) {
	//	glog.V(logger.Info).Infof("ask is this msg=%d", msg.Code)
	// read the header
	headbuf := make([]byte, 32)
	if _, err := io.ReadFull(rw.conn, headbuf); err != nil {
		return msg, err
	}
	// verify header mac 验证标头 mac
	shouldMAC := updateMAC(rw.ingressMAC, rw.macCipher, headbuf[:16])
	if !hmac.Equal(shouldMAC, headbuf[16:]) {
		return msg, errors.New("bad header MAC")
	}
	rw.dec.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now decrypted 前半部分现已解密
	fsize := readInt24(headbuf)
	// ignore protocol type for now

	// read the frame content 读取框架内容
	var rsize = fsize // frame size rounded up to 16 byte boundary 帧大小四舍五入到 16 字节边界
	if padding := fsize % 16; padding > 0 {
		rsize += 16 - padding
	}
	framebuf := make([]byte, rsize)
	if _, err := io.ReadFull(rw.conn, framebuf); err != nil {
		return msg, err
	}

	// read and validate frame MAC. we can re-use headbuf for that. 读取并验证帧 MAC。 我们可以为此重用headbuf
	rw.ingressMAC.Write(framebuf)
	fmacseed := rw.ingressMAC.Sum(nil)
	if _, err := io.ReadFull(rw.conn, headbuf[:16]); err != nil {
		return msg, err
	}
	shouldMAC = updateMAC(rw.ingressMAC, rw.macCipher, fmacseed)
	if !hmac.Equal(shouldMAC, headbuf[:16]) {
		return msg, errors.New("bad frame MAC")
	}

	// decrypt frame content
	rw.dec.XORKeyStream(framebuf, framebuf)

	// decode message code
	content := bytes.NewReader(framebuf[:fsize])
	if err := rlp.Decode(content, &msg.Code); err != nil {
		return msg, err
	}
	msg.Size = uint32(content.Len())
	msg.Payload = content
	//	glog.V(logger.Info).Infof("ask ask is bu is this msg=%d", msg.Code)
	return msg, nil
}

// updateMAC reseeds the given hash with encrypted seed.
// it returns the first 16 bytes of the hash sum after seeding. updateMAC 使用加密种子重新播种给定的哈希。 它在播种后返回哈希和的前 16 个字节
func updateMAC(mac hash.Hash, block cipher.Block, seed []byte) []byte {
	aesbuf := make([]byte, aes.BlockSize)
	block.Encrypt(aesbuf, mac.Sum(nil))
	for i := range aesbuf {
		aesbuf[i] ^= seed[i]
	}
	mac.Write(aesbuf)
	return mac.Sum(nil)[:16]
}

func readInt24(b []byte) uint32 {
	return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16
}

func putInt24(v uint32, b []byte) {
	b[0] = byte(v >> 16)
	b[1] = byte(v >> 8)
	b[2] = byte(v)
}
