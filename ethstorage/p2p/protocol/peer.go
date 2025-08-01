// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"context"
	"math"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Peer is a collection of relevant information we have about a `storage` peer.
type Peer struct {
	id             peer.ID // Unique ID for the peer, cached
	newStreamFn    newStreamFn
	chainId        *big.Int
	direction      network.Direction
	version        uint                        // Protocol version negotiated
	shards         map[common.Address][]uint64 // shards of this node support
	minRequestSize float64
	tracker        *Tracker
	resCtx         context.Context
	resCancel      context.CancelFunc
	logger         log.Logger // Contextual logger with the peer id injected
}

// NewPeer create a wrapper for a network connection and negotiated  protocol version.
func NewPeer(version uint, chainId *big.Int, peerId peer.ID, newStream newStreamFn, direction network.Direction,
	initRequestSize, minRequestSize uint64, shards map[common.Address][]uint64) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Peer{
		id:             peerId,
		newStreamFn:    newStream,
		chainId:        chainId,
		direction:      direction,
		version:        version,
		shards:         shards,
		minRequestSize: float64(minRequestSize),
		tracker:        NewTracker(peerId.String(), float64(initRequestSize)/(p2pReadWriteTimeout.Seconds()*rttEstimateFactor)),
		resCtx:         ctx,
		resCancel:      cancel,
		logger:         log.New("peer", peerId[:8]),
	}
}

// ID retrieves the peer's unique identifier.
func (p *Peer) ID() peer.ID {
	return p.id
}

// Version retrieves the peer's negotiated `storage` protocol version.
func (p *Peer) Version() uint {
	return p.version
}

func (p *Peer) Shards() map[common.Address][]uint64 {
	return p.shards
}

// IsShardExist checks whether one specific shard is supported by this peer.
func (p *Peer) IsShardExist(contract common.Address, shardId uint64) bool {
	if ids, ok := p.shards[contract]; ok {
		if slices.Contains(ids, shardId) {
			return true
		}
	}

	return false
}

// Log overrides the P2P logger with the higher level one containing only the id.
func (p *Peer) Log() log.Logger {
	return p.logger
}

func (p *Peer) getRequestSize() uint64 {
	return uint64(math.Max(p.tracker.Capacity(p2pReadWriteTimeout.Seconds()*rttEstimateFactor), p.minRequestSize))
}

// RequestBlobsByRange fetches a batch of kvs using a list of kv index
func (p *Peer) RequestBlobsByRange(id uint64, contract common.Address, shardId uint64, origin uint64, limit uint64,
	blobs *BlobsByRangePacket) (byte, error) {
	p.logger.Trace("Fetching KVs", "reqId", id, "contract", contract,
		"shardId", shardId, "origin", origin, "limit", limit)

	ctx, cancel := context.WithTimeout(p.resCtx, NewStreamTimeout)
	defer cancel()

	stream, err := p.newStreamFn(ctx, p.id, GetProtocolID(RequestBlobsByRangeProtocolID, p.chainId))
	if err != nil {
		return streamError, err
	}
	defer func() {
		if stream != nil {
			stream.Close()
		}
	}()

	requestSize := p.getRequestSize()
	return SendRPC(stream, &GetBlobsByRangePacket{
		ID:       id,
		Contract: contract,
		ShardId:  shardId,
		Origin:   origin,
		Limit:    limit,
		Bytes:    requestSize,
	}, blobs)
}

// RequestBlobsByList fetches a batch of kvs using a list of kv index
func (p *Peer) RequestBlobsByList(id uint64, contract common.Address, shardId uint64, kvList []uint64,
	blobs *BlobsByListPacket) (byte, error) {
	p.logger.Trace("Fetching KVs", "reqId", id, "contract", contract,
		"shardId", shardId, "count", len(kvList))

	ctx, cancel := context.WithTimeout(p.resCtx, NewStreamTimeout)
	defer cancel()

	stream, err := p.newStreamFn(ctx, p.id, GetProtocolID(RequestBlobsByListProtocolID, p.chainId))
	if err != nil {
		return streamError, err
	}
	defer func() {
		if stream != nil {
			stream.Close()
		}
	}()

	requestSize := p.getRequestSize()
	return SendRPC(stream, &GetBlobsByListPacket{
		ID:       id,
		Contract: contract,
		ShardId:  shardId,
		BlobList: kvList,
		Bytes:    requestSize,
	}, blobs)
}
