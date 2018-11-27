package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"strconv"
	"time"

	"github.com/icon-project/goloop/common"
	"github.com/icon-project/goloop/common/crypto"
	"github.com/icon-project/goloop/module"
)

type transactionV3JSON struct {
	Version   common.HexUint16 `json:"version"` // V3 only
	From      common.Address   `json:"from"`
	To        common.Address   `json:"to"`
	Value     common.HexUint   `json:"value"`
	StepLimit common.HexUint   `json:"stepLimit"` // V3 only
	Fee       common.HexUint   `json:"fee"`       // V2 only
	TimeStamp common.HexUint64 `json:"timestamp"`
	NID       common.HexUint16 `json:"nid"` // V3 only
	Nonce     common.HexUint64 `json:"nonce"`
	TxHash    common.HexBytes  `json:"txHash"`  // V3 only
	Tx_Hash   common.HexBytes  `json:"tx_hash"` // V2 only
	Signature common.Signature `json:"signature"`

	DataType string          `json:"dataType"`
	Data     json.RawMessage `json:"data"`

	raw    []byte
	txHash []byte
}

var (
	v2FieldInclusion = map[string]bool(nil)
	v2FieldExclusion = map[string]bool{
		"method":    true,
		"signature": true,
		"tx_hash":   true,
	}
	v3FieldInclusion = map[string]bool(nil)
	v3FieldExclusion = map[string]bool{
		"signature": true,
		"txHash":    true,
	}
)

func (tx *transactionV3JSON) calcHash() ([]byte, error) {
	var data map[string]interface{}
	var err error
	if err = json.Unmarshal(tx.raw, &data); err != nil {
		return nil, err
	}
	var bs []byte
	if tx.Version.Value == 2 {
		bs, err = SerializeMap(data, v2FieldInclusion, v2FieldExclusion)
	} else {
		bs, err = SerializeMap(data, v3FieldInclusion, v3FieldExclusion)
	}
	if err != nil {
		log.Println("Serialize FAILs")
		log.Println("JSON", string(tx.raw))
		return nil, err
	}
	bs = append([]byte("icx_sendTransaction."), bs...)

	return crypto.SHA3Sum256(bs), nil
}

func (tx *transactionV3JSON) ID() []byte {
	if err := tx.updateTxHash(); err != nil {
		log.Panicf("Fail to calculate TxHash err=%+v", err)
	}
	return tx.txHash
}

func (tx *transactionV3JSON) updateTxHash() error {
	if tx.txHash == nil {
		h, err := tx.calcHash()
		if err != nil {
			return err
		}
		tx.txHash = h
	}
	return nil
}

func (tx *transactionV3JSON) verifySignature() error {
	pk, err := tx.Signature.RecoverPublicKey(tx.txHash)
	if err != nil {
		return err
	}
	addr := common.NewAccountAddressFromPublicKey(pk)
	if addr.Equal(&tx.From) {
		return nil
	}
	return errors.New("InvalidSignature")
}

func (tx *transactionV3JSON) Timestamp() int64 {
	return int64(tx.TimeStamp.Value)
}

type transactionV3 struct {
	*transactionV3JSON
	hash []byte
}

var stepsForTransfer = big.NewInt(100000)

func (tx *transactionV3) Version() int {
	return module.TransactionVersion3
}

func (tx *transactionV3) Verify() error {
	if tx.DataType == "" {
		if tx.StepLimit.Cmp(stepsForTransfer) < 0 {
			return ErrNotEnoughStep
		}
	} else {
		if !tx.To.IsContract() {
			return ErrContractIsRequired
		}
	}

	if err := tx.updateTxHash(); err != nil {
		return err
	}

	if len(tx.TxHash) > 0 && !bytes.Equal(tx.txHash, tx.TxHash) {
		return ErrInvalidHashValue
	}

	if err := tx.transactionV3JSON.verifySignature(); err != nil {
		return err
	}

	return nil
}

func (tx *transactionV3) PreValidate(wc WorldContext, update bool) error {
	stepPrice := wc.StepPrice()

	trans := new(big.Int)
	trans.Set(&tx.StepLimit.Int)
	trans.Mul(trans, stepPrice)
	trans.Add(trans, &tx.Value.Int)

	as1 := wc.GetAccountState(tx.From.ID())
	balance1 := as1.GetBalance()
	if balance1.Cmp(trans) < 0 {
		return ErrNotEnoughBalance
	}

	if configOnCheckingTimestamp {
		tsdiff := wc.TimeStamp() - int64(tx.TimeStamp.Value)
		if tsdiff < int64(-5*time.Minute/time.Microsecond) ||
			tsdiff > int64(5*time.Minute/time.Microsecond) {
			return ErrTimeOut
		}
	}

	if update {
		as2 := wc.GetAccountState(tx.To.ID())
		balance2 := as2.GetBalance()
		balance2.Add(balance2, &tx.Value.Int)
		balance1.Sub(balance1, trans)
		as1.SetBalance(balance1)
		as2.SetBalance(balance2)
	}
	return nil
}

func (tx *transactionV3) Prepare(wvs WorldVirtualState) (WorldVirtualState, error) {
	var lq []LockRequest
	if !tx.To.IsContract() {
		lq = []LockRequest{
			{string(tx.From.ID()), AccountWriteLock},
			{string(tx.To.ID()), AccountWriteLock},
		}
	} else {
		lq = []LockRequest{
			{"", AccountWriteLock},
		}
	}
	return wvs.GetFuture(lq), nil
}

func (tx *transactionV3) transfer(wc WorldContext,
	from, to module.Address, value, limit *big.Int) (bool, interface{}, *big.Int) {
	stepUse := big.NewInt(wc.StepsFor(StepTypeDefault, 1))

	if stepUse.Cmp(limit) > 0 {
		return false, FailureOutOfStep, limit
	}

	as1 := wc.GetAccountState(from.ID())
	bal1 := as1.GetBalance()
	if bal1.Cmp(value) < 0 {
		return false, FailureOutOfBalance, limit
	}
	bal1.Sub(bal1, value)
	as1.SetBalance(bal1)

	as2 := wc.GetAccountState(to.ID())
	bal2 := as2.GetBalance()
	bal2.Add(bal2, value)
	as2.SetBalance(bal2)

	return true, nil, stepUse
}

func (tx *transactionV3) call(wc WorldContext, r Receipt,
	from, to module.Address, value, limit *big.Int) (bool, interface{}, *big.Int) {
	// TODO Implement transaction v3 contract call
	log.Panicf("Not implemented")
	return false, FailureUnknown, &zero
}

func countBytesOfData(data interface{}) int {
	switch o := data.(type) {
	case string:
		if len(o) > 2 && o[:2] == "0x" {
			o = o[2:]
		}
		bs := []byte(o)
		for _, b := range bs {
			if (b < '0' || b > '9') && (b < 'a' || b > 'f') {
				return len(bs)
			}
		}
		return (len(bs) + 1) / 2
	case []interface{}:
		var count int
		for _, i := range o {
			count += countBytesOfData(i)
		}
		return count
	case map[string]interface{}:
		var count int
		for _, i := range o {
			count += countBytesOfData(i)
		}
		return count
	case bool:
		return 1
	case float64:
		return len(common.Int64ToBytes(int64(o)))
	default:
		return 0
	}
}

var zero big.Int

func (tx *transactionV3) Execute(wc WorldContext) (Receipt, error) {
	r := NewReceipt(&tx.To)
	stepPrice := wc.StepPrice()
	var (
		fee                big.Int
		success            bool
		result             interface{}
		stepUsed, bal1     *big.Int
		stepUse, stepLimit big.Int
	)
	wcs := wc.GetSnapshot()
	as1 := wc.GetAccountState(tx.From.ID())
	stepLimit.Set(&tx.StepLimit.Int)

	// it tries to execute
	if tx.To.IsContract() {
		success, result, stepUsed = tx.call(wc, r, &tx.From, &tx.To, &tx.Value.Int, &stepLimit)
		stepUse.Set(stepUsed)
		stepLimit.Sub(&stepLimit, stepUsed)
	} else {
		success, result, stepUsed = tx.transfer(wc, &tx.From, &tx.To, &tx.Value.Int, &stepLimit)
		stepUse.Set(stepUsed)
		stepLimit.Sub(&stepLimit, stepUsed)
		if success && tx.DataType == "message" {
			var data interface{}
			if err := json.Unmarshal(tx.Data, &data); err != nil {
				success = false
				result = FailureUnknown
				stepUsed = &stepLimit
			} else {
				var stepsForMessage big.Int
				stepsForMessage.SetInt64(wc.StepsFor(StepTypeInput, countBytesOfData(data)))
				if stepLimit.Cmp(&stepsForMessage) < 0 {
					success = false
					result = FailureOutOfStepForInput
					stepUsed = &stepLimit
				} else {
					stepUsed = &stepsForMessage
				}
			}
			stepUse.Add(&stepUse, stepUsed)
			stepLimit.Sub(&stepLimit, stepUsed)
		}
	}

	// try to charge fee
	fee.Mul(&stepUse, stepPrice)
	bal1 = as1.GetBalance()
	for bal1.Cmp(&fee) < 0 {
		if success {
			// rollback all changes
			success = false
			result = FailureNotPayable
			wc.Reset(wcs)
			r = NewReceipt(&tx.To)
			bal1 = as1.GetBalance()

			stepUse.Set(&tx.StepLimit.Int)
			fee.Mul(&stepUse, stepPrice)
		} else {
			stepPrice = &zero
			fee.SetInt64(0)
		}
	}
	bal1.Sub(bal1, &fee)
	as1.SetBalance(bal1)

	r.SetResult(success, result, &stepUse, stepPrice)
	return r, nil
}

func (tx *transactionV3) Group() module.TransactionGroup {
	return module.TransactionGroupNormal
}

func (tx *transactionV3) Bytes() []byte {
	return tx.raw
}

func (tx *transactionV3) Hash() []byte {
	if tx.hash == nil {
		tx.hash = crypto.SHA3Sum256(tx.Bytes())
	}
	return tx.hash
}

func (tx *transactionV3) ToJSON(version int) (interface{}, error) {
	if version == module.TransactionVersion3 {
		var jso map[string]interface{}
		if err := json.Unmarshal(tx.raw, &jso); err != nil {
			return nil, err
		}
		return jso, nil
	} else {
		return nil, errors.New("InvalidVersion:" + strconv.Itoa(version))
	}
}

func (tx *transactionV3) MarshalJSON() ([]byte, error) {
	return tx.raw, nil
}

func newTransactionV2V3FromJSON(js []byte) (Transaction, error) {
	genjs := new(genesisV3JSON)
	if err := json.Unmarshal(js, genjs); err != nil {
		return nil, err
	}
	if len(genjs.Accounts) != 0 {
		genjs.raw = make([]byte, len(js))
		copy(genjs.raw, js)

		return &genesisV3{genesisV3JSON: genjs}, nil
	}

	txjs := new(transactionV3JSON)
	txjs.Version.Value = 2
	if err := json.Unmarshal(js, txjs); err != nil {
		return nil, err
	}
	txjs.raw = make([]byte, len(js))
	copy(txjs.raw, js)

	switch txjs.Version.Value {
	case 2:
		return &transactionV2{transactionV3JSON: txjs}, nil
	case 3:
		return &transactionV3{transactionV3JSON: txjs}, nil
	default:
		return nil, errors.New("IllegalVersion:" + txjs.Version.String())
	}
}
