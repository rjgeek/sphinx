package event

import (
	"github.com/shx-project/sphinx/blockchain/types"
	"github.com/shx-project/sphinx/common"
)

type RemovedLogsEvent struct {
	Logs []*types.Log
}
type ChainEvent struct {
	Block *types.Block
	Hash  common.Hash
	Logs  []*types.Log
}
type ChainHeadEvent struct {
	Message *types.Block
}
type ChainSideEvent struct {
	Block *types.Block
}
type LogsEvent struct {
	Logs []*types.Log
}
type TxPreEvent struct {
	Message *types.Transaction
}