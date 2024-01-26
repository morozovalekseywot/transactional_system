package helpers

import (
	"bwg_transactional_system/internal/broker"
	"bwg_transactional_system/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hashicorp/go-set"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"math/rand"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func MustMarshal(v any) []byte {
	bytes, err := json.Marshal(v)
	failOnError(err, "can't marshall")

	return bytes
}

type Producer struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	msgs  <-chan amqp.Delivery
	qName string
}

func NewProducer(cfg *broker.Config) *Producer {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%s/", cfg.Username, cfg.Password, cfg.Host, cfg.Port))
	failOnError(err, "Failed to connect to RabbitMQ")

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")

	err = ch.ExchangeDeclare(
		"queries", // name
		"topic",   // type
		false,     // durable
		false,     // auto-deleted
		false,     // internal
		false,     // no-wait
		nil,       // arguments
	)
	failOnError(err, "Failed to declare an exchange")

	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		false, // delete when unused
		true,  // exclusive
		false, // noWait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	// регистрируем приёмник сообщений
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	return &Producer{
		conn:  conn,
		ch:    ch,
		msgs:  msgs,
		qName: q.Name,
	}
}

func (p *Producer) Invoice(ctx context.Context, id string, req models.InvoiceRequest) {
	err := p.ch.PublishWithContext(ctx,
		"queries",                // exchange
		string(broker.OpInvoice), // routing key
		false,                    // mandatory
		false,                    // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: id,
			ReplyTo:       p.qName,
			Body:          MustMarshal(req),
		})
	failOnError(err, "Failed to publish a message")
}

func (p *Producer) Withdraw(ctx context.Context, id string, req models.WithdrawRequest) {
	err := p.ch.PublishWithContext(ctx,
		"queries",                 // exchange
		string(broker.OpWithdraw), // routing key
		false,                     // mandatory
		false,                     // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: id,
			ReplyTo:       p.qName,
			Body:          MustMarshal(req),
		})
	failOnError(err, "Failed to publish a message")
}

func (p *Producer) GetBalance(ctx context.Context, id string, req models.GetBalanceRequest) {
	err := p.ch.PublishWithContext(ctx,
		"queries",                   // exchange
		string(broker.OpGetBalance), // routing key
		false,                       // mandatory
		false,                       // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: id,
			ReplyTo:       p.qName,
			Body:          MustMarshal(req),
		})
	failOnError(err, "Failed to publish a message")
}

func (p *Producer) Close() {
	failOnError(p.ch.Close(), "can't close channel")
	failOnError(p.conn.Close(), "can't close connection")
}

func RunTest1(cfg *broker.Config) {
	producer := NewProducer(cfg)

	ctx := context.Background()
	log.Print("Begin producing messages")
	n := 5
	invoiceIDs := set.New[string](n)
	for i := 0; i < n; i++ {
		id := uuid.NewString()
		invoiceIDs.Insert(id)
		producer.Invoice(ctx, id, models.InvoiceRequest{
			WalletID: 1,
			Ticker:   "USD",
			Amount:   1,
		})
	}

	log.Print("Wait for invoice's responses")
	for d := range producer.msgs {
		if invoiceIDs.Contains(d.CorrelationId) {
			log.Printf("Message: %s", d.Body)
			invoiceIDs.Remove(d.CorrelationId)
			if invoiceIDs.Empty() {
				break
			}
		}
	}

	log.Print("Produce withdraw request")
	withDrawID := uuid.NewString()
	producer.Withdraw(ctx, withDrawID, models.WithdrawRequest{
		WalletID: 1,
		Ticker:   "USD",
		Amount:   50,
	})

	log.Print("Wait for withdraw response")
	for d := range producer.msgs {
		if d.CorrelationId == withDrawID {
			log.Printf("Message: %s", d.Body)
			break
		}
	}

	log.Print("Produce balance request")
	balanceID := uuid.NewString()
	producer.GetBalance(ctx, balanceID, models.GetBalanceRequest{WalletID: 1})

	log.Print("Wait for balance response")
	for d := range producer.msgs {
		if d.CorrelationId == balanceID {
			log.Printf("Message: %s", d.Body)
			break
		}
	}
	log.Print("All responses received")
}

func RunTest2(cfg *broker.Config) {
	producer := NewProducer(cfg)

	ctx := context.Background()
	log.Print("Begin producing messages")
	n1 := 30
	invoiceIDs := set.New[string](n1)
	for i := 0; i < n1; i++ {
		id := uuid.NewString()
		invoiceIDs.Insert(id)
		go producer.Invoice(ctx, id, models.InvoiceRequest{
			WalletID: 1 + rand.Intn(8),
			Ticker:   "USD",
			Amount:   1,
		})
	}

	log.Print("Produce withdraw request")
	n2 := 40
	withdrawIDs := set.New[string](n2)
	for i := 0; i < n2; i++ {
		id := uuid.NewString()
		withdrawIDs.Insert(id)
		go producer.Withdraw(ctx, id, models.WithdrawRequest{
			WalletID: 1 + rand.Intn(8),
			Ticker:   "USD",
			Amount:   1,
		})
	}

	log.Print("Produce balance request")
	n3 := 50
	getBalancesIDs := set.New[string](n3)
	for i := 0; i < n3; i++ {
		id := uuid.NewString()
		getBalancesIDs.Insert(id)
		go producer.GetBalance(ctx, id, models.GetBalanceRequest{WalletID: 1 + rand.Intn(8)})
	}

	log.Print("Wait for responses")
	count := 0
	for d := range producer.msgs {
		log.Printf("Message: %s", d.Body)
		count++
		if count >= n1+n2+n3 {
			break
		}
	}

	log.Print("All responses received")
}
