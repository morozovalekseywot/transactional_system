package broker

import (
	"context"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Operation string

const (
	OpInvoice    Operation = "invoice"
	OpWithdraw   Operation = "withdraw"
	OpGetBalance Operation = "balance"
)

var Operations = []Operation{OpInvoice, OpWithdraw, OpGetBalance}

type Broker interface {
	SendError(ctx context.Context, err error, d *amqp.Delivery)
	SendSuccess(ctx context.Context, d *amqp.Delivery)
	SendResponse(ctx context.Context, bytes []byte, d *amqp.Delivery)
	RunConsumer(ctx context.Context, invoiceOp, withDrawOp, BalanceOp func(context.Context, *amqp.Delivery))
	Close() error
}

type ErrorResponse struct {
	Operation string `json:"operation"`
	Code      int    `json:"code"`
	Reason    error  `json:"reason"`
}

type SuccessResponse struct {
	Operation string `json:"operation"`
	Code      int    `json:"code"`
	Body      []byte `json:"body,omitempty"`
}
