/*
 * Copyright (C) 2018 The ontology Authors
 * This file is part of The ontology library.
 *
 * The ontology is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The ontology is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with The ontology.  If not, see <http://www.gnu.org/licenses/>.
 */
package connect_controller

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ontio/ontology/p2pserver/handshake"
	"github.com/ontio/ontology/p2pserver/peer"
	"github.com/stretchr/testify/assert"
	"github.com/ontio/ontology/p2pserver/common"
)

func init() {
	common.Difficulty = 1
	handshake.HANDSHAKE_DURATION = 10 * time.Second
}

type Transport struct {
	dialer     Dialer
	listener   net.Listener
	listenAddr string
	t          *testing.T
}

func NewTransport(t *testing.T) *Transport {
	listener, err := net.Listen("tcp", "127.0.0.1:")
	assert.Nil(t, err)
	assert.NotNil(t, listener)

	return &Transport{
		t:          t,
		listenAddr: listener.Addr().String(),
		listener:   listener,
		dialer:     &noTlsDialer{},
	}
}

func (self *Transport) Accept() net.Conn {
	conn, err := self.listener.Accept()
	assert.Nil(self.t, err)
	return conn
}

func (self *Transport) Pipe() (net.Conn, net.Conn) {
	c := make(chan net.Conn)
	go func() {
		conn, err := self.listener.Accept()
		assert.Nil(self.t, err)
		c <- conn
	}()
	client, err := self.dialer.Dial(self.listenAddr)
	assert.Nil(self.t, err)

	server := <-c

	return client, server
}

type Node struct {
	*ConnectController
	Info *peer.PeerInfo
	Key  *common.PeerKeyId
}

func NewNode(option ConnCtrlOption) *Node {
	key := common.RandPeerKeyId()
	info := &peer.PeerInfo{
		Id:          key.Id,
		Port:        20338,
		SoftVersion: "v1.9.0-beta",
	}

	return &Node{
		ConnectController: NewConnectController(info, key, option),
		Info:              info,
		Key:               key,
	}
}

func TestConnectController_CanDetectSelfAddress(t *testing.T) {
	versions := []string{"v1.8.0", "v1.7.0", "v1.9.0", "v1.9.0-beta", "v1.20"}
	for _, version := range versions {
		trans := NewTransport(t)
		server := NewNode(NewConnCtrlOption())
		server.Info.SoftVersion = version
		assert.Equal(t, server.OwnAddress(), "")

		c, s := trans.Pipe()
		go func() {
			_, _ = handshake.HandshakeClient(server.peerInfo, server.Key, c)
		}()

		_, _, err := server.AcceptConnect(s)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "handshake with itself")

		assert.Equal(t, server.OwnAddress(), "127.0.0.1:20338")
	}
}

func TestConnectController_AcceptConnect_MaxInBound(t *testing.T) {
	trans := NewTransport(t)
	maxInboud := 5
	server := NewNode(NewConnCtrlOption().MaxInBound(uint(maxInboud)))
	client := NewNode(NewConnCtrlOption().MaxOutBound(uint(maxInboud * 2)))

	clientConns := make(chan net.Conn, maxInboud)
	for i := 0; i < maxInboud*2; i++ {
		conn1, conn2 := trans.Pipe()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := handshake.HandshakeClient(client.peerInfo, client.Key, conn1)
			if i < int(maxInboud) {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		}(i)
		checkServer(t, client, server, clientConns, i, conn2, maxInboud, false)
		wg.Wait()
	}

	close(clientConns)
	for conn := range clientConns {
		_ = conn.Close()
	}

	assert.Equal(t, server.inoutbounds[INBOUND_INDEX].Size(), 0)
}

func TestConnectController_OutboundsCount(t *testing.T) {
	maxOutboud := 5
	server := NewNode(NewConnCtrlOption().MaxInBound(uint(maxOutboud * 2)))
	client := NewNode(NewConnCtrlOption().MaxOutBound(uint(maxOutboud)))

	clientConns := make(chan net.Conn, maxOutboud*2)
	for i := 0; i < maxOutboud*2; i++ {
		trans := NewTransport(t)
		go func(i int, trans *Transport) {
			con := trans.Accept()
			checkServer(t, client, server, clientConns, i, con, maxOutboud, true)
		}(i, trans)
		_, _, err := client.Connect(trans.listenAddr)
		if i < maxOutboud {
			assert.Nil(t, err)
			assert.Equal(t, client.boundsCount(OUTBOUND_INDEX), uint(i+1))
		} else {
			assert.NotNil(t, err)
		}
	}

	assert.Equal(t, client.boundsCount(OUTBOUND_INDEX), uint(maxOutboud))
	for i := 0; i < maxOutboud; i++ {
		conn := <-clientConns
		_ = conn.Close()
	}

	assert.Equal(t, server.boundsCount(INBOUND_INDEX), uint(0))
}

func TestConnCtrlOption_MaxInBoundPerIp(t *testing.T) {
	trans := NewTransport(t)
	maxInBoundPerIp := 5
	server := NewNode(NewConnCtrlOption().MaxInBoundPerIp(uint(maxInBoundPerIp)))
	client := NewNode(NewConnCtrlOption().MaxInBoundPerIp(uint(maxInBoundPerIp)))

	clientConns := make(chan net.Conn, maxInBoundPerIp)
	for i := 0; i < maxInBoundPerIp*2; i++ {
		conn1, conn2 := trans.Pipe()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := handshake.HandshakeClient(client.peerInfo, client.Key, conn1)
			if i < int(maxInBoundPerIp) {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		}(i)

		checkServer(t, client, server, clientConns, i, conn2, maxInBoundPerIp, false)
		wg.Wait()
	}

	close(clientConns)
	for conn := range clientConns {
		_ = conn.Close()
	}

	assert.Equal(t, server.inoutbounds[INBOUND_INDEX].Size(), 0)
}

func checkServer(t *testing.T, client, server *Node, clientConns chan<- net.Conn, i int, conn2 net.Conn, maxLimit int, isCheck bool) {
	info, conn, err := server.AcceptConnect(conn2)
	if i >= maxLimit && isCheck == false {
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "reach max limit")
		_ = conn2.Close()
		return
	}
	assert.Nil(t, err)
	info.Addr = "" // client.Info is not set
	assert.Equal(t, info, client.Info)

	clientConns <- conn
}

func TestCheckReserveWithDomain(t *testing.T) {
	a := assert.New(t)
	dname := "www.baidu.com"
	gips, err := net.LookupHost(dname)
	a.Nil(err, "fail to get domain record")

	cc := &ConnectController{}
	cc.ReservedPeers = []string{dname}
	for _, ip := range gips {
		err := cc.checkReservedPeers(ip)
		a.Nil(err, "fail")
	}
}
