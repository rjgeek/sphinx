// Copyright 2018 The sphinx Authors
// Modified based on go-ethereum, which Copyright (C) 2014 The go-ethereum Authors.
//
// The sphinx is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The sphinx is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the sphinx. If not, see <http://www.gnu.org/licenses/>.

// Contains the meters and timers used by the networking layer.

package p2p

import (
	"github.com/shx-project/sphinx/common/metrics"
	"net"
)

var (
	ingressConnectMeter = metrics.NewMeter("p2p/InboundConnects")
	ingressTrafficMeter = metrics.NewMeter("p2p/InboundTraffic")
	egressConnectMeter  = metrics.NewMeter("p2p/OutboundConnects")
	egressTrafficMeter  = metrics.NewMeter("p2p/OutboundTraffic")
)

type meteredConn struct {
	*net.TCPConn
}

func newMeteredConn(conn net.Conn, ingress bool) net.Conn {
	// Short circuit if metrics are disabled
	if !metrics.Enabled {
		return conn
	}
	// Otherwise bump the connection counters and wrap the connection
	if ingress {
		ingressConnectMeter.Mark(1)
	} else {
		egressConnectMeter.Mark(1)
	}
	return &meteredConn{conn.(*net.TCPConn)}
}

func (c *meteredConn) Read(b []byte) (n int, err error) {
	n, err = c.TCPConn.Read(b)
	ingressTrafficMeter.Mark(int64(n))
	return
}

func (c *meteredConn) Write(b []byte) (n int, err error) {
	n, err = c.TCPConn.Write(b)
	egressTrafficMeter.Mark(int64(n))
	return
}

////////////////////////////////////////////////////////////////////////////////////////////////////

var (
	propTxnInPacketsMeter     = metrics.NewMeter("shx/prop/txns/in/packets")
	propTxnInTrafficMeter     = metrics.NewMeter("shx/prop/txns/in/traffic")
	propTxnOutPacketsMeter    = metrics.NewMeter("shx/prop/txns/out/packets")
	propTxnOutTrafficMeter    = metrics.NewMeter("shx/prop/txns/out/traffic")
	propHashInPacketsMeter    = metrics.NewMeter("shx/prop/hashes/in/packets")
	propHashInTrafficMeter    = metrics.NewMeter("shx/prop/hashes/in/traffic")
	propHashOutPacketsMeter   = metrics.NewMeter("shx/prop/hashes/out/packets")
	propHashOutTrafficMeter   = metrics.NewMeter("shx/prop/hashes/out/traffic")
	propBlockInPacketsMeter   = metrics.NewMeter("shx/prop/blocks/in/packets")
	propBlockInTrafficMeter   = metrics.NewMeter("shx/prop/blocks/in/traffic")
	propBlockOutPacketsMeter  = metrics.NewMeter("shx/prop/blocks/out/packets")
	propBlockOutTrafficMeter  = metrics.NewMeter("shx/prop/blocks/out/traffic")
	reqHeaderInPacketsMeter   = metrics.NewMeter("shx/req/headers/in/packets")
	reqHeaderInTrafficMeter   = metrics.NewMeter("shx/req/headers/in/traffic")
	reqHeaderOutPacketsMeter  = metrics.NewMeter("shx/req/headers/out/packets")
	reqHeaderOutTrafficMeter  = metrics.NewMeter("shx/req/headers/out/traffic")
	reqBodyInPacketsMeter     = metrics.NewMeter("shx/req/bodies/in/packets")
	reqBodyInTrafficMeter     = metrics.NewMeter("shx/req/bodies/in/traffic")
	reqBodyOutPacketsMeter    = metrics.NewMeter("shx/req/bodies/out/packets")
	reqBodyOutTrafficMeter    = metrics.NewMeter("shx/req/bodies/out/traffic")
	reqStateInPacketsMeter    = metrics.NewMeter("shx/req/states/in/packets")
	reqStateInTrafficMeter    = metrics.NewMeter("shx/req/states/in/traffic")
	reqStateOutPacketsMeter   = metrics.NewMeter("shx/req/states/out/packets")
	reqStateOutTrafficMeter   = metrics.NewMeter("shx/req/states/out/traffic")
	reqReceiptInPacketsMeter  = metrics.NewMeter("shx/req/receipts/in/packets")
	reqReceiptInTrafficMeter  = metrics.NewMeter("shx/req/receipts/in/traffic")
	reqReceiptOutPacketsMeter = metrics.NewMeter("shx/req/receipts/out/packets")
	reqReceiptOutTrafficMeter = metrics.NewMeter("shx/req/receipts/out/traffic")
	miscInPacketsMeter        = metrics.NewMeter("shx/misc/in/packets")
	miscInTrafficMeter        = metrics.NewMeter("shx/misc/in/traffic")
	miscOutPacketsMeter       = metrics.NewMeter("shx/misc/out/packets")
	miscOutTrafficMeter       = metrics.NewMeter("shx/misc/out/traffic")
)

type meteredMsgReadWriter struct {
	MsgReadWriter
	version uint
}

func newMeteredMsgWriter(rw MsgReadWriter) MsgReadWriter {
	if !metrics.Enabled {
		return rw
	}
	return &meteredMsgReadWriter{MsgReadWriter: rw}
}

func (rw *meteredMsgReadWriter) Init(version uint) {
	rw.version = version
}

func (rw *meteredMsgReadWriter) ReadMsg() (Msg, error) {
	// Read the message and short circuit in case of an error
	msg, err := rw.MsgReadWriter.ReadMsg()
	if err != nil {
		return msg, err
	}
	// Account for the data traffic
	packets, traffic := miscInPacketsMeter, miscInTrafficMeter
	switch {
	case msg.Code == BlockHeadersMsg:
		packets, traffic = reqHeaderInPacketsMeter, reqHeaderInTrafficMeter
	case msg.Code == BlockBodiesMsg:
		packets, traffic = reqBodyInPacketsMeter, reqBodyInTrafficMeter

	case msg.Code == NodeDataMsg:
		packets, traffic = reqStateInPacketsMeter, reqStateInTrafficMeter
	case msg.Code == ReceiptsMsg:
		packets, traffic = reqReceiptInPacketsMeter, reqReceiptInTrafficMeter

	case msg.Code == NewBlockHashesMsg:
		packets, traffic = propHashInPacketsMeter, propHashInTrafficMeter
	case msg.Code == NewBlockMsg:
		packets, traffic = propBlockInPacketsMeter, propBlockInTrafficMeter
	case msg.Code == TxMsg:
		packets, traffic = propTxnInPacketsMeter, propTxnInTrafficMeter
	case msg.Code == WorkProofMsg:
		packets, traffic = propTxnInPacketsMeter, propTxnInTrafficMeter
	}
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	return msg, err
}

func (rw *meteredMsgReadWriter) WriteMsg(msg Msg) error {
	// Account for the data traffic
	packets, traffic := miscOutPacketsMeter, miscOutTrafficMeter
	switch {
	case msg.Code == BlockHeadersMsg:
		packets, traffic = reqHeaderOutPacketsMeter, reqHeaderOutTrafficMeter
	case msg.Code == BlockBodiesMsg:
		packets, traffic = reqBodyOutPacketsMeter, reqBodyOutTrafficMeter

	case msg.Code == NodeDataMsg:
		packets, traffic = reqStateOutPacketsMeter, reqStateOutTrafficMeter
	case msg.Code == ReceiptsMsg:
		packets, traffic = reqReceiptOutPacketsMeter, reqReceiptOutTrafficMeter

	case msg.Code == NewBlockHashesMsg:
		packets, traffic = propHashOutPacketsMeter, propHashOutTrafficMeter
	case msg.Code == NewBlockMsg:
		packets, traffic = propBlockOutPacketsMeter, propBlockOutTrafficMeter
	case msg.Code == TxMsg:
		packets, traffic = propTxnOutPacketsMeter, propTxnOutTrafficMeter
	case msg.Code == WorkProofMsg:
		packets, traffic = propTxnOutPacketsMeter, propTxnOutTrafficMeter
	case msg.Code == ProofConfirmMsg:
		packets, traffic = propTxnOutPacketsMeter, propTxnOutTrafficMeter
	}
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	return rw.MsgReadWriter.WriteMsg(msg)
}
