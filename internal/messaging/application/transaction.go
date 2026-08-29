package application

import "context"

// TransactionRunner is supplied by the composition root so a multi-copy
// message operation commits its facts, outbox rows, and media bindings as one
// database transaction without exposing database details to Messaging.
type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}

type directTransactionRunner struct{}

func (directTransactionRunner) Run(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}
