package broker

import (
	"context"
	"encoding/json"
	amqp "github.com/rabbitmq/amqp091-go"
	"net/http"
)

type Operation string

const (
	OpInvoice    Operation = "invoice"
	OpWithdraw   Operation = "withdraw"
	OpGetBalance Operation = "balance"
)

var Operations = []Operation{OpInvoice, OpWithdraw, OpGetBalance}

type Broker interface {
	SendResponse(ctx context.Context, bytes []byte, d *amqp.Delivery)
	RunConsumer(ctx context.Context, invoiceOp, withDrawOp, BalanceOp func(context.Context, *amqp.Delivery))
	Close() error
}

type ErrorResponse struct {
	Operation string `json:"operation"`
	Code      int    `json:"code"`
	Reason    string `json:"reason"`
}

type SuccessResponse struct {
	Operation string `json:"operation"`
	Code      int    `json:"code"`
	Body      string `json:"body,omitempty"`
}

func NewBadRequestResponse(d *amqp.Delivery, err error) []byte {
	resp, _ := json.Marshal(ErrorResponse{
		Code:      http.StatusBadRequest,
		Reason:    err.Error(),
		Operation: d.RoutingKey,
	})
	return resp
}

func NewErrorResponse(d *amqp.Delivery, httpStatus int, err error) []byte {
	resp, _ := json.Marshal(ErrorResponse{
		Code:      httpStatus,
		Reason:    err.Error(),
		Operation: d.RoutingKey,
	})
	return resp
}

func NewSuccessResponse(d *amqp.Delivery) []byte {
	resp, _ := json.Marshal(SuccessResponse{
		Code:      http.StatusOK,
		Operation: d.RoutingKey,
	})
	return resp
}

func NewSuccessResponseWithBody(d *amqp.Delivery, body []byte) []byte {
	resp, _ := json.Marshal(SuccessResponse{
		Code:      http.StatusOK,
		Operation: d.RoutingKey,
		Body:      string(body),
	})
	return resp
}
