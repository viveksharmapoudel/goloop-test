package server

import (
	"fmt"

	"github.com/labstack/echo/v4"

	"github.com/icon-project/goloop/common"
	"github.com/icon-project/goloop/module"
	"github.com/icon-project/goloop/server/jsonrpc"
)

type BlockRequest struct {
	Height       common.HexInt64 `json:"height"`
	EventFilters []*EventFilter  `json:"eventFilters,omitempty"`
	bn           BlockNotification
}

type BlockNotification struct {
	Hash    common.HexBytes     `json:"hash"`
	Height  common.HexInt64     `json:"height"`
	Indexes [][]common.HexInt32 `json:"indexes,omitempty"`
}

func (wm *wsSessionManager) RunBlockSession(ctx echo.Context) error {
	var br BlockRequest
	wss, err := wm.initSession(ctx, &br)
	if err != nil {
		return err
	}
	defer wm.StopSession(wss)

	if err := br.compile(); err != nil {
		_ = wss.response(int(jsonrpc.ErrorCodeInvalidParams), err.Error())
		return nil
	}
	_ = wss.response(0, "")

	ech := make(chan error)
	go readLoop(wss.c, ech)

	h := br.Height.Value
	var bch <-chan module.Block
	bm := wss.chain.BlockManager()
	sm := wss.chain.ServiceManager()
	indexes := make([][]common.HexInt32, len(br.EventFilters))
	for i := range br.EventFilters {
		indexes[i] = make([]common.HexInt32, 0)
	}
	var rl module.ReceiptList
loop:
	for {
		bch, err = bm.WaitForBlock(h)
		if err != nil {
			break loop
		}
		select {
		case err = <-ech:
			break loop
		case blk := <-bch:
			br.bn.Height = common.HexInt64{Value: h}
			br.bn.Hash = blk.ID()
			if rl != nil {
				rl = nil
			}
			if len(br.bn.Indexes) > 0 {
				br.bn.Indexes = indexes[:0]
				for i := range indexes {
					indexes[i] = indexes[i][:0]
				}
			}
			lb := blk.LogsBloom()
			for i, f := range br.EventFilters {
				if lb.Contain(f.lb) {
					if rl == nil {
						rl, err = sm.ReceiptListFromResult(blk.Result(), module.TransactionGroupNormal)
						if err != nil {
							break loop
						}
					}
					index := int32(0)
					for rit := rl.Iterator(); rit.Has(); rit.Next() {
						r, err := rit.Get()
						if err != nil {
							break loop
						}
						if f.match(r) {
							if len(br.bn.Indexes) < 1 {
								br.bn.Indexes = indexes[:]
							}
							br.bn.Indexes[i] = append(br.bn.Indexes[i], common.HexInt32{Value: index})
						}
						index++
					}
				}
			}
			if err = wss.WriteJSON(&br.bn); err != nil {
				wm.logger.Infof("fail to write json BlockNotification err:%+v\n", err)
				break loop
			}
		}
		h++
	}
	wm.logger.Warnf("%+v\n", err)
	return nil
}

func (r *BlockRequest) compile() error {
	if len(r.EventFilters) < 1 {
		return fmt.Errorf("event filters cannot empty")
	}
	for i, f := range r.EventFilters {
		if err := f.compile(); err != nil {
			return fmt.Errorf("fail to compile idx:%d, err:%v", i, err)
		}
	}
	return nil
}

