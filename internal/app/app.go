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

// sendBadRequest отправляет ответ с ошибкой связанной с неправильными данными в запросе
func (a *App) sendBadRequest(ctx context.Context, op broker.Operation, err error, d *amqp.Delivery) {
	a.Broker.SendResponse(ctx, broker.NewBadRequestResponse(d, err), d)
	accountMetrics(op, http.StatusBadRequest)
}

// sendSuccess отправляет сообщение с успешным результатом операции
func (a *App) sendSuccess(ctx context.Context, op broker.Operation, d *amqp.Delivery) {
	a.Broker.SendResponse(ctx, broker.NewSuccessResponse(d), d)
	accountMetrics(op, http.StatusOK)
}

func (a *App) invoiceOperation(ctx context.Context, d *amqp.Delivery) {
	req := models.InvoiceRequest{}
	if err := json.Unmarshal(d.Body, &req); err != nil {
		a.sendBadRequest(ctx, broker.OpInvoice, err, d)
		return
	}
	if err := req.Validate(); err != nil {
		a.sendBadRequest(ctx, broker.OpInvoice, err, d)
		return
	}

	// отправляем запрос в базу данных
	err := a.Repo.Invoice(ctx, &req)
	a.processResult(ctx, broker.OpInvoice, err, d)
}

func (a *App) withdrawOperation(ctx context.Context, d *amqp.Delivery) {
	req := models.WithdrawRequest{}
	if err := json.Unmarshal(d.Body, &req); err != nil {
		a.sendBadRequest(ctx, broker.OpWithdraw, err, d)
		return
	}
	if err := req.Validate(); err != nil {
		a.sendBadRequest(ctx, broker.OpWithdraw, err, d)
		return
	}

	// отправляем запрос в базу данных
	err := a.Repo.WithDraw(ctx, &req)
	a.processResult(ctx, broker.OpWithdraw, err, d)
}

func (a *App) processResult(ctx context.Context, op broker.Operation, err error, d *amqp.Delivery) {
	if err != nil {
		var e repository.LogicErrors
		switch {
		case errors.As(err, &e):
			a.sendBadRequest(ctx, op, e, d)
		default:
			a.Broker.SendResponse(ctx, broker.NewErrorResponse(d, http.StatusInternalServerError, err), d)
			accountMetrics(op, http.StatusInternalServerError)
		}
	} else {
		a.sendSuccess(ctx, op, d)
	}
}

func (a *App) getBalanceOperation(ctx context.Context, d *amqp.Delivery) {
	req := models.GetBalanceRequest{}
	if err := json.Unmarshal(d.Body, &req); err != nil {
		a.sendBadRequest(ctx, broker.OpGetBalance, err, d)
		return
	}

	// отправляем запрос в базу данных
	resp, err := a.Repo.GetBalance(ctx, &req)
	if err != nil {
		a.processResult(ctx, broker.OpGetBalance, err, d)
		return
	}

	bytes, _ := json.Marshal(resp)
	body, _ := json.Marshal(broker.SuccessResponse{
		Operation: d.RoutingKey,
		Code:      http.StatusOK,
		Body:      string(bytes),
	})
	a.Broker.SendResponse(ctx, body, d)
	accountMetrics(broker.OpGetBalance, http.StatusOK)
}
