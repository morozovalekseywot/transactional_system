package broker

import (
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"net/http"
	"sync"
)

const ExchangeName = "queries"

type Config struct {
	Port     string
	Host     string
	Username string
	Password string
}

type RabbitMQ struct {
	conn *amqp.Connection
	ch   *amqp.Channel
	msgs <-chan amqp.Delivery
	wg   *sync.WaitGroup
	stop chan struct{}
}

func NewRabbitMQ(cfg *Config) (*RabbitMQ, error) {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%s/", cfg.Username, cfg.Password, cfg.Host, cfg.Port))
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open a channel: %v", err)
	}

	err = ch.ExchangeDeclare(
		ExchangeName, // name
		"topic",      // type
		false,        // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare an exchange: %v", err)
	}

	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare a queue: %v", err)
	}

	// чтобы новое сообщение отправлялось обработчику, только после ответа на предыдущее
	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set QoS: %v", err)
	}

	// Добавляем топик по имени каждой возможной операции
	for _, op := range Operations {
		err = ch.QueueBind(
			q.Name,       // queue name
			string(op),   // routing key
			ExchangeName, // exchange
			false,
			nil)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, err
	}

	return &RabbitMQ{
		conn: conn,
		ch:   ch,
		msgs: msgs,
		wg:   &sync.WaitGroup{},
		stop: make(chan struct{}),
	}, nil
}

// SendError отправляет ответ с ошибкой связанной с неправильными данными в запросе
func (b *RabbitMQ) SendError(ctx context.Context, err error, d *amqp.Delivery) {
	resp, _ := json.Marshal(ErrorResponse{Code: http.StatusBadRequest, Reason: err.Error(), Operation: d.RoutingKey})
	b.SendResponse(ctx, resp, d)
}

// SendError отправляет сообщение с успешным результатом операции
func (b *RabbitMQ) SendSuccess(ctx context.Context, d *amqp.Delivery) {
	resp, _ := json.Marshal(SuccessResponse{Code: http.StatusOK, Operation: d.RoutingKey})
	b.SendResponse(ctx, resp, d)
}

// SendResponse отправляет сообщение с body = bytes
func (b *RabbitMQ) SendResponse(ctx context.Context, bytes []byte, d *amqp.Delivery) {
	err := b.ch.PublishWithContext(ctx,
		"",        // exchange
		d.ReplyTo, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: d.CorrelationId,
			Body:          bytes,
		})
	if err != nil {
		log.Printf("Error when publish message: %v", err)
	}
	log.Printf("Send response to: %s with body: %s", d.ReplyTo, bytes)
}

// RunConsumer запускает ещё одного обработчика запросов, приходящих через брокера
func (b *RabbitMQ) RunConsumer(ctx context.Context, invoiceOp, withDrawOp, BalanceOp func(context.Context, *amqp.Delivery)) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case d := <-b.msgs:
				log.Printf("Get message, routing key: %s, body: %s", d.RoutingKey, d.Body)
				switch Operation(d.RoutingKey) {
				case OpInvoice:
					invoiceOp(ctx, &d)
				case OpWithdraw:
					withDrawOp(ctx, &d)
				case OpGetBalance:
					BalanceOp(ctx, &d)
				default:
					b.SendError(ctx, fmt.Errorf("no such operation: %s", d.RoutingKey), &d)
				}

				d.Ack(false)
			case <-b.stop:
				return
			}
		}
	}()
}

func (b *RabbitMQ) Close() error {
	// stop reader
	close(b.stop)
	b.wg.Wait()

	if err := b.ch.Close(); err != nil {
		return err
	}
	if err := b.conn.Close(); err != nil {
		return err
	}

	return nil
}
