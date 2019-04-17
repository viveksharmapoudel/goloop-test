package service

import "github.com/icon-project/goloop/common/errors"

const (
	DuplicateTransactionError errors.Code = iota + errors.CodeService
	TransactionPoolOverflowError
	ExpiredTransactionError
	TransitionInterruptedError
)

var (
	ErrDuplicateTransaction    = errors.NewBase(DuplicateTransactionError, "DuplicateTransaction")
	ErrTransactionPoolOverFlow = errors.NewBase(TransactionPoolOverflowError, "TransactionPoolOverFlow")
	ErrExpiredTransaction      = errors.NewBase(ExpiredTransactionError, "ExpiredTransaction")
	ErrTransitionInterrupted   = errors.NewBase(TransitionInterruptedError, "TransitionInterrupted")
)
