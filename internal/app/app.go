package app

import (
	"bwg_transactional_system/internal/broker"
	"bwg_transactional_system/internal/models"
	"bwg_transactional_system/internal/repository"
	"context"
	"encoding/json"
	"errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"net/http"
)

type App struct {
	Repo   repository.Repository
	Broker broker.Broker
}

func NewApp(repo repository.Repository, broker broker.Broker) *App {
	return &App{Repo: repo, Broker: broker}
}

func (a *App) RunConsumer(ctx context.Context) {
	a.Broker.RunConsumer(ctx, a.invoiceOperation, a.withdrawOperation, a.getBalanceOperation)
}

func (a *App) Close() error {
	// сначала останавливаем брокера сообщений
	err := a.Broker.Close()
	if err != nil {
		return err
	}

	err = a.Repo.Close()
	if err != nil {
		return err
	}

	return nil
}

func (a *App) invoiceOperation(ctx context.Context, d *amqp.Delivery) {
	req := models.InvoiceRequest{}
	if err := json.Unmarshal(d.Body, &req); err != nil {
		a.Broker.SendError(ctx, err, d)
		return
	}
	if err := req.Validate(); err != nil {
		a.Broker.SendError(ctx, err, d)
		return
	}

	// отправляем запрос в базу данных
	err := a.Repo.Invoice(ctx, &req)
	a.processResult(ctx, err, d)
}

func (a *App) withdrawOperation(ctx context.Context, d *amqp.Delivery) {
	req := models.WithdrawRequest{}
	if err := json.Unmarshal(d.Body, &req); err != nil {
		a.Broker.SendError(ctx, err, d)
		return
	}
	if err := req.Validate(); err != nil {
		a.Broker.SendError(ctx, err, d)
		return
	}

	// отправляем запрос в базу данных
	err := a.Repo.WithDraw(ctx, &req)
	a.processResult(ctx, err, d)
}

func (a *App) processResult(ctx context.Context, err error, d *amqp.Delivery) {
	if err != nil {
		var e repository.LogicErrors
		switch {
		case errors.As(err, &e):
			a.Broker.SendError(ctx, e, d)
		default:
			body, _ := json.Marshal(broker.ErrorResponse{Code: http.StatusInternalServerError, Reason: err.Error(), Operation: d.RoutingKey})
			a.Broker.SendResponse(ctx, body, d)
		}
	} else {
		a.Broker.SendSuccess(ctx, d)
	}
}

func (a *App) getBalanceOperation(ctx context.Context, d *amqp.Delivery) {
	req := models.GetBalanceRequest{}
	if err := json.Unmarshal(d.Body, &req); err != nil {
		a.Broker.SendError(ctx, err, d)
		return
	}

	// отправляем запрос в базу данных
	resp, err := a.Repo.GetBalance(ctx, &req)
	if err != nil {
		a.processResult(ctx, err, d)
		return
	}
	bytes, _ := json.Marshal(resp)
	body, _ := json.Marshal(broker.SuccessResponse{
		Operation: d.RoutingKey,
		Code:      http.StatusOK,
		Body:      string(bytes),
	})
	a.Broker.SendResponse(ctx, body, d)
}
