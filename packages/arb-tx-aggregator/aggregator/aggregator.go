/*
 * Copyright 2020, Offchain Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package aggregator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/offchainlabs/arbitrum/packages/arb-evm/message"
	"log"
	"math/big"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/offchainlabs/arbitrum/packages/arb-evm/evm"
	"github.com/offchainlabs/arbitrum/packages/arb-tx-aggregator/batcher"
	"github.com/offchainlabs/arbitrum/packages/arb-tx-aggregator/txdb"
	"github.com/offchainlabs/arbitrum/packages/arb-util/common"
	"github.com/offchainlabs/arbitrum/packages/arb-util/inbox"
	"github.com/offchainlabs/arbitrum/packages/arb-util/machine"
	"github.com/offchainlabs/arbitrum/packages/arb-util/value"
	"github.com/offchainlabs/arbitrum/packages/arb-validator-core/ethutils"
)

type Server struct {
	client          ethutils.EthClient
	chain           common.Address
	batch           *batcher.Batcher
	db              *txdb.TxDB
	callMach        machine.Machine
	pendingCallMach machine.Machine
	callBlock       *common.BlockId
	maxCallTime     time.Duration
	maxCallGas      *big.Int
}

// NewServer returns a new instance of the Server class
func NewServer(
	ctx context.Context,
	client ethutils.EthClient,
	batch *batcher.Batcher,
	rollupAddress common.Address,
	db *txdb.TxDB,
) (*Server, error) {
	mach, block := db.CallInfo()
	server := &Server{
		client:          client,
		chain:           rollupAddress,
		batch:           batch,
		db:              db,
		callMach:        mach,
		pendingCallMach: mach,
		callBlock:       block,
		maxCallTime:     0,
		maxCallGas:      big.NewInt(100000000),
	}

	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mach, block := db.CallInfo()
				if mach.Hash() == server.callMach.Hash() {
					// The machine hasn't changed so updating is easy
					server.callBlock = block
				} else {
					server.callMach = mach

					//_, currentCount, err := globalInbox.GetInbox(ctx)
					//if err != nil {
					//	return
					//}
					//inbox := server.batch.PendingMessages(currentCount)

				}
				break
			}
		}
	}()

	return server, nil
}

// SendTransaction takes a request signed transaction l2message from a client
// and puts it in a queue to be included in the next transaction batch
func (m *Server) SendTransaction(_ context.Context, tx *types.Transaction) (common.Hash, error) {
	return m.batch.SendTransaction(tx)
}

//FindLogs takes a set of parameters and return the list of all logs that match
//the query
func (m *Server) FindLogs(ctx context.Context, fromHeight, toHeight *uint64, addresses []ethcommon.Address, topics [][]ethcommon.Hash) ([]evm.FullLog, error) {
	topicGroups := make([][]common.Hash, 0)
	for _, group := range topics {
		topicGroups = append(topicGroups, common.HashArrayFromEth(group))
	}

	return m.db.FindLogs(
		ctx,
		fromHeight,
		toHeight,
		common.AddressArrayFromEth(addresses),
		topicGroups,
	)
}

func (m *Server) GetBlockCount(_ context.Context) (uint64, error) {
	id, err := m.db.LatestBlock()
	if err != nil {
		return 0, err
	}
	return id.Height.AsInt().Uint64(), nil
}

func (m *Server) GetOutputMessage(
	_ context.Context,
	args *evm.GetOutputMessageArgs,
	reply *evm.GetOutputMessageReply,
) error {
	msg, err := m.db.GetMessage(args.Index)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := value.MarshalValue(msg, &buf); err != nil {
		return err
	}
	reply.RawVal = hexutil.Encode(buf.Bytes())
	return nil
}

// GetMessageResult returns the value output by the VM in response to the
//l2message with the given hash
func (m *Server) GetRequestResult(_ context.Context, requestId common.Hash) (value.Value, error) {
	return m.db.GetRequest(requestId)
}

// GetVMInfo returns current metadata about this VM
func (m *Server) GetChainAddress(_ context.Context) (ethcommon.Address, error) {
	return m.chain.ToEthAddress(), nil
}

func (m *Server) BlockInfo(_ context.Context, height uint64) (*machine.BlockInfo, error) {
	return m.db.GetBlock(height)
}

func (m *Server) GetBlockHeader(ctx context.Context, height uint64) (*types.Header, error) {
	currentBlock, err := m.db.GetBlock(height)
	if err != nil {
		return nil, err
	}

	ethHeader, err := m.client.HeaderByHash(ctx, currentBlock.Hash.ToEthHash())
	if err != nil {
		return nil, err
	}

	gasUsed := uint64(0)
	gasLimit := uint64(100000000)
	if currentBlock != nil {
		res, err := evm.NewBlockResultFromValue(currentBlock.BlockLog)
		if err != nil {
			return nil, err
		}
		gasUsed = res.BlockStats.GasUsed.Uint64()
		gasLimit = res.GasLimit.Uint64()
	}

	ethHeader.Coinbase = common.RandAddress().ToEthAddress()
	ethHeader.Bloom = currentBlock.Bloom
	ethHeader.GasLimit = gasLimit
	ethHeader.GasUsed = gasUsed

	return ethHeader, nil
}

func (m *Server) GetBlock(ctx context.Context, height uint64) (*types.Block, error) {
	header, err := m.GetBlockHeader(ctx, height)
	if err != nil {
		return nil, err
	}
	currentBlock, err := m.db.GetBlock(height)
	if err != nil {
		return nil, err
	}

	if currentBlock == nil {
		// No arbitrum block at this height
		return types.NewBlock(header, nil, nil, nil), nil
	}

	res, err := evm.NewBlockResultFromValue(currentBlock.BlockLog)
	if err != nil {
		return nil, err
	}
	txCount := res.BlockStats.TxCount.Uint64()
	startLog := res.FirstAVMLog().Uint64()
	transactions := make([]*types.Transaction, 0, txCount)
	receipts := make([]*types.Receipt, 0, txCount)
	for i := uint64(0); i < txCount; i++ {
		avmLog, err := m.db.GetLog(startLog + i)
		if err != nil {
			return nil, err
		}
		res, err := evm.NewTxResultFromValue(avmLog)
		if err != nil {
			return nil, err
		}
		receipt, err := res.ToEthReceipt(currentBlock.Hash)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)

		tx, err := GetTransaction(res.L1Message, m.chain)
		if err == nil {
			transactions = append(transactions, tx)
		}
	}

	return types.NewBlock(header, transactions, nil, receipts), nil
}

func (m *Server) GetTransaction(_ context.Context, requestId ethcommon.Hash) (*types.Transaction, error) {
	val, err := m.db.GetRequest(common.NewHashFromEth(requestId))
	if err != nil {
		return nil, nil
	}
	res, err := evm.NewTxResultFromValue(val)
	if err != nil {
		return nil, err
	}
	return GetTransaction(res.L1Message, m.chain)
}

func GetTransaction(msg inbox.InboxMessage, chain common.Address) (*types.Transaction, error) {
	if msg.Kind != message.L2Type {
		return nil, errors.New("result is not a transaction")
	}
	l2msg, err := message.L2Message{Data: msg.Data}.AbstractMessage()
	if err != nil {
		return nil, err
	}
	ethMsg, ok := l2msg.(message.EthConvertable)
	if !ok {
		return nil, errors.New("message not convertible to receipt")
	}
	return ethMsg.AsEthTx(chain)
}

// Call takes a request from a client to process in a temporary context
// and return the result
func (m *Server) Call(ctx context.Context, msg message.ContractTransaction, sender ethcommon.Address) (value.Value, error) {
	mach, blockId := m.db.CallInfo()
	return m.executeCall(mach.Clone(), blockId, msg, sender)
}

// PendingCall takes a request from a client to process in a temporary context
// and return the result
func (m *Server) PendingCall(ctx context.Context, msg message.ContractTransaction, sender ethcommon.Address) (value.Value, error) {
	mach, blockId := m.db.CallInfo()
	return m.executeCall(mach.Clone(), blockId, msg, sender)
}

func (m *Server) executeCall(mach machine.Machine, blockId *common.BlockId, msg message.ContractTransaction, sender ethcommon.Address) (value.Value, error) {
	seq, _ := new(big.Int).SetString("999999999999999999999999", 10)
	if msg.MaxGas.Cmp(big.NewInt(0)) == 0 || msg.MaxGas.Cmp(m.maxCallGas) > 0 {
		msg.MaxGas = m.maxCallGas
	}
	log.Println("Executing call", msg.MaxGas, msg.GasPriceBid, msg.DestAddress, msg.Payment)
	inboxMsg := message.NewInboxMessage(
		message.NewL2Message(msg),
		common.NewAddressFromEth(sender),
		seq,
		inbox.ChainTime{
			BlockNum:  blockId.Height,
			Timestamp: big.NewInt(time.Now().Unix()),
		},
	)

	assertion, steps := mach.ExecuteAssertion(
		// Call execution is only limited by wall time, so use a massive max steps as an approximation to infinity
		10000000000000000,
		[]inbox.InboxMessage{inboxMsg},
		m.maxCallTime,
	)

	// If the machine wasn't able to run and it reports that it is currently
	// blocked, return the block reason to give the client more information
	// as opposed to just returning a general "call produced no output"
	if br := mach.IsBlocked(true); steps == 0 && br != nil {
		log.Println("can't produce solution since machine is blocked", br)
		return nil, fmt.Errorf("can't produce solution since machine is blocked %v", br)
	}

	log.Println("Executed call for", steps, "steps")

	results := assertion.ParseLogs()
	if len(results) == 0 {
		return nil, errors.New("call produced no output")
	}
	lastLogVal := results[len(results)-1]
	lastLog, err := evm.NewTxResultFromValue(lastLogVal)
	if err != nil {
		return nil, err
	}
	if lastLog.L1Message.MessageID() != inboxMsg.MessageID() {
		// Last produced log is not the call we sent
		return nil, errors.New("call took too long to execute")
	}
	return results[len(results)-1], nil
}
