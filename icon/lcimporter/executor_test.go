/*
 * Copyright 2021 ICON Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package lcimporter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/icon-project/goloop/common/db"
	"github.com/icon-project/goloop/common/log"
)

const delayForConfirm = 10*time.Millisecond

func buildTestTx(height int64, suffix string) *BlockTransaction {
	return &BlockTransaction{
		Height:        height,
		BlockID:       []byte(fmt.Sprintf("BLOCKID[%d,%s]", height, suffix)),
		Result:        []byte(fmt.Sprintf("RESULT[%d,%s]", height, suffix)),
		ValidatorHash: []byte(fmt.Sprintf("VALIDATOR[%d,%s]", height, suffix)),
		TXCount:       TransactionsPerBlock/6,
	}
}

func buildTestTxs(from, to int64, suffix string) []*BlockTransaction {
	txs := make([]*BlockTransaction, 0, int(to-from+1))
	for height := from; height <= to; height += 1 {
		tx := buildTestTx(height, suffix)
		txs = append(txs, tx)
	}
	return txs
}

type testBCRequest struct {
	from, to int64
	txs      []*BlockTransaction
	channel  chan interface{}
}

func (r *testBCRequest) sendTxs(txs []*BlockTransaction) {
	for _, tx := range txs {
		r.channel <- tx
	}
}

func (r *testBCRequest) generateTxs(from, to int64, suffix string) {
	for height := from; height <= to; height += 1 {
		tx := buildTestTx(height, suffix)
		r.channel <- tx
	}
}

type testBlockConverter struct {
	channel chan *testBCRequest
}

func (t *testBlockConverter) Rebase(from, to int64, txs []*BlockTransaction) (<-chan interface{}, error) {
	req := &testBCRequest{
		from:    from,
		to:      to,
		txs:     txs,
		channel: make(chan interface{}, 1),
	}
	t.channel <- req
	return req.channel, nil
}

func newTestBlockConverter(rdb db.Database) *testBlockConverter {
	return &testBlockConverter{ make(chan *testBCRequest, 1)}
}

func TestExecutor_Basic(t *testing.T) {
	rdb := db.NewMapDB()
	idb := db.NewMapDB()
	logger := log.GlobalLogger()
	bc := newTestBlockConverter(rdb)
	ex, err := NewExecutorWithBC(rdb, idb, logger, bc)
	assert.NoError(t, err)

	t.Log("start executor")
	err = ex.Start()
	assert.NoError(t, err)
	t.Log("executor started")

	toTC := make(chan string, 3)
	toBC := make(chan string, 3)

	txs1 := buildTestTxs(0, 9, "OK")
	go func() {
		req := <- bc.channel
		t.Log("request received")
		assert.Equal(t, int64(0), req.from)
		assert.Equal(t, int64(0), req.to)
		assert.Nil(t, req.txs)
		t.Log("sending 0~4")
		req.sendTxs(txs1[0:5])

		time.Sleep(delayForConfirm)
		toTC <- "on_send5"
		assert.Equal(t, "on_timeout", <-toBC)

		t.Log("sending 5~9")
		req.sendTxs(txs1[5:])

		assert.Equal(t, "quit", <-toBC)
		close(req.channel)
	}()
	_, err = ex.GetTransactions(0, 9, func(txs []*BlockTransaction, err error) {
		t.Logf("transaction arrives size=%d", len(txs))
		assert.NoError(t, err)
		assert.Equal(t, txs1, txs)

		toTC <- "on_receive_10"
	})
	assert.NoError(t, err)
	assert.Equal(t, "on_send5", <-toTC)
	select {
	case <-time.After(delayForConfirm):
		// do nothing
		toBC <- "on_timeout"
	case msg := <-toTC:
		assert.Failf(t, "unexpected msg", "msg=%s", msg)
	}
	assert.Equal(t, "on_receive_10", <-toTC)

	toBC <- "quit"
	time.Sleep(delayForConfirm)
	ex.Term()
}

func TestExecutor_Propose(t *testing.T) {
	rdb := db.NewMapDB()
	idb := db.NewMapDB()
	logger := log.GlobalLogger()
	bc := newTestBlockConverter(rdb)
	ex, err := NewExecutorWithBC(rdb, idb, logger, bc)
	assert.NoError(t, err)

	t.Log("start executor")
	err = ex.Start()
	assert.NoError(t, err)
	t.Log("executor started")

	toTC := make(chan string, 3)
	toBC := make(chan string, 3)

	txs1 := buildTestTxs(0, 9, "OK")
	go func() {
		req := <- bc.channel
		t.Log("request received")
		assert.Equal(t, int64(0), req.from)
		assert.Equal(t, int64(0), req.to)
		assert.Nil(t, req.txs)
		t.Log("sending 0~4")
		req.sendTxs(txs1[0:5])

		time.Sleep(delayForConfirm)
		toTC <- "on_send_5"
		assert.Equal(t, "send_remain", <-toBC)

		t.Log("sending 5~9")
		req.sendTxs(txs1[5:])

		time.Sleep(delayForConfirm)
		toTC <- "on_send_10"
		close(req.channel)
	}()

	assert.Equal(t, "on_send_5", <-toTC)
	txs, err := ex.ProposeTransactions()
	assert.NoError(t, err)
	assert.Equal(t, txs1[0:5], txs)
	toBC <- "send_remain"

	err = ex.FinalizeTransactions(4)
	assert.NoError(t, err)

	assert.Equal(t, "on_send_10", <-toTC)
	txs, err = ex.ProposeTransactions()
	assert.NoError(t, err)
	assert.Equal(t, txs1[5:], txs)

	err = ex.FinalizeTransactions(9)
	assert.NoError(t, err)

	ex.Term()

	t.Log("continue from 10")
	ex, err = NewExecutorWithBC(rdb, idb, logger, bc)
	assert.NoError(t, err)
	err = ex.Start()
	assert.NoError(t, err)

	go func() {
		req := <-bc.channel
		assert.Equal(t, int64(10), req.from)
		assert.Equal(t, int64(0), req.to)
		assert.Nil(t, req.txs)

		t.Log("sending 5 more")
		txs := buildTestTxs(req.from, req.from+4, "OK")
		req.sendTxs(txs)
		time.Sleep(delayForConfirm)
		toTC <- "on_send_15"

		assert.Equal(t, "quit", <-toBC)
		close(req.channel)
	}()

	assert.Equal(t, "on_send_15", <-toTC)
	t.Log("propose and finalize to=14")
	txs2, err := ex.ProposeTransactions()
	assert.NoError(t, err)
	assert.Equal(t, 5, len(txs2))
	err = ex.FinalizeTransactions(14)
	assert.NoError(t, err)

	t.Log("cleanup")
	toBC <- "quit"
	ex.Term()
}

func TestExecutor_SyncTransactions(t *testing.T) {
	rdb := db.NewMapDB()
	idb := db.NewMapDB()
	logger := log.GlobalLogger()
	bc := newTestBlockConverter(rdb)
	ex, err := NewExecutorWithBC(rdb, idb, logger, bc)
	assert.NoError(t, err)

	t.Log("start executor")
	err = ex.Start()
	assert.NoError(t, err)
	t.Log("executor started")

	toTest := make(chan string, 3)
	toBC := make(chan string, 3)

	txs1 := buildTestTxs(0, 9, "OK")
	txs2 := buildTestTxs(0, 9, "OTHER")
	go func() {
		req := <- bc.channel
		t.Log("request received")
		assert.Equal(t, int64(0), req.from)
		assert.Equal(t, int64(0), req.to)
		assert.Nil(t, req.txs)
		t.Log("sending 0~9")
		req.sendTxs(txs1[0:5])

		assert.Equal(t, "send_old_remain", <-toBC)

		req.sendTxs(txs1[5:])
		close(req.channel)
	}()

	t.Log("try to get 0~9 (should block)")

	_, err = ex.GetTransactions(0, 9, func(txs []*BlockTransaction, err error) {
		assert.Error(t, err)

		toTest <- "on_failure_for_previous"
	})

	t.Log("try to get 0~4 (trigger failure on previous request)")

	_, err = ex.GetTransactions(0, 4, func(txs []*BlockTransaction, err error) {
		assert.NoError(t, err)
		assert.Equal(t, txs1[0:5], txs)

		toTest <- "on_receive_old_5"
	})

	msgs := append([]string{}, <-toTest)
	msgs = append(msgs, <-toTest)
	t.Log("check failure of get(0~9)")
	assert.Contains(t, msgs, "on_failure_for_previous")

	t.Log("check result of get(0~4)")
	assert.Contains(t, msgs, "on_receive_old_5")

	t.Log("try to get 1~4 (should success)")
	_, err = ex.GetTransactions(1, 4, func(txs []*BlockTransaction, err error) {
		assert.NoError(t, err)
		assert.Equal(t, txs1[1:5], txs)

		toTest <- "confirm_1~4"
	})

	t.Log("check result of get(1~4)")
	assert.Equal(t, "confirm_1~4", <-toTest)

	go func() {
		req := <- bc.channel
		t.Log("sync request received")
		assert.Equal(t, int64(0), req.from)
		assert.Equal(t, int64(0), req.to)
		assert.Equal(t, txs2[0:5], req.txs)

		req.sendTxs(txs2)

		close(req.channel)
	}()

	err = ex.SyncTransactions(txs2[0:5])
	assert.NoError(t, err)

	toBC <- "send_old_remain"

	_, err = ex.GetTransactions(0, 4, func(txs []*BlockTransaction, err error) {
		t.Log("receive new 5")
		assert.NoError(t, err)
		assert.Equal(t, txs2[0:5], txs)

		toTest <- "on_receive_new_5"
	})
	assert.Equal(t, "on_receive_new_5", <-toTest)

	t.Log("finalize to=4")
	err = ex.FinalizeTransactions(4)
	assert.NoError(t, err)

	canceler, err := ex.GetTransactions(5, 10, func(txs []*BlockTransaction, err error) {
		t.Logf("canceled err=%v", err)
		assert.Error(t, err)

		toTest <- "on_expected_failure"
	})
	assert.NoError(t, err)

	canceler()

	select {
	case <- time.After(time.Millisecond*100):
		assert.Fail(t, "Timeout to receive result")
	case v := <-toTest:
		assert.Equal(t, "on_expected_failure", v)
	}
	ex.Term()
}

func TestExecutor_Term(t *testing.T) {
	rdb := db.NewMapDB()
	idb := db.NewMapDB()
	logger := log.GlobalLogger()
	bc := newTestBlockConverter(rdb)
	ex, err := NewExecutorWithBC(rdb, idb, logger, bc)
	assert.NoError(t, err)

	t.Log("start executor")
	err = ex.Start()
	assert.NoError(t, err)
	t.Log("executor started")

	toTest := make(chan string, 3)
	toBC := make(chan string, 3)

	txs1 := buildTestTxs(0, 9, "OK")
	go func() {
		req := <- bc.channel
		t.Log("request received")
		assert.Equal(t, int64(0), req.from)
		assert.Equal(t, int64(0), req.to)
		assert.Nil(t, req.txs)
		t.Log("sending 0~8")
		req.sendTxs(txs1[:9])

		assert.Equal(t, "send_after_term", <-toBC)

		t.Log("sending 9")
		req.sendTxs(txs1[9:])
		close(req.channel)

		toTest<-"closed"
	}()

	_, err = ex.GetTransactions(0, 9, func(txs []*BlockTransaction, err error) {
		assert.Error(t, err)
		toTest <- "got_error"
	})

	ex.Term()
	assert.Equal(t, "got_error", <-toTest)

	toBC <- "send_after_term"

	assert.Equal(t, "closed", <-toTest)
	time.Sleep(delayForConfirm)
}