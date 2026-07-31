package service

import "sync/atomic"

type AccessVersionMetrics struct {
	initializations      atomic.Uint64
	unexpectedMissing    atomic.Uint64
	tokenIssuanceMissing atomic.Uint64
	increments           atomic.Uint64
	postIncrementMissing atomic.Uint64
	transactionRetries   atomic.Uint64
	deadlockRetries      atomic.Uint64
}

type AccessVersionMetricsSnapshot struct {
	Initializations      uint64
	UnexpectedMissing    uint64
	TokenIssuanceMissing uint64
	Increments           uint64
	PostIncrementMissing uint64
	TransactionRetries   uint64
	DeadlockRetries      uint64
}

func NewAccessVersionMetrics() *AccessVersionMetrics {
	return &AccessVersionMetrics{}
}

func (m *AccessVersionMetrics) Snapshot() AccessVersionMetricsSnapshot {
	return AccessVersionMetricsSnapshot{
		Initializations:      m.initializations.Load(),
		UnexpectedMissing:    m.unexpectedMissing.Load(),
		TokenIssuanceMissing: m.tokenIssuanceMissing.Load(),
		Increments:           m.increments.Load(),
		PostIncrementMissing: m.postIncrementMissing.Load(),
		TransactionRetries:   m.transactionRetries.Load(),
		DeadlockRetries:      m.deadlockRetries.Load(),
	}
}
