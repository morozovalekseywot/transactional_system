package main

import (
	"bwg_transactional_system/internal/app"
	"bwg_transactional_system/internal/broker"
	"bwg_transactional_system/internal/models"
	"bwg_transactional_system/internal/repository"
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"time"

	"github.com/google/uuid"
	set "github.com/hashicorp/go-set"
	// _ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"os"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func newInvoice(walletID int, ticker string, amount float32) []byte {
	bytes, _ := json.Marshal(models.InvoiceRequest{
		WalletID: walletID,
		Ticker:   ticker,
		Amount:   amount,
	})

	return bytes
}

func newWithdraw(walletID int, ticker string, amount float32) []byte {
	bytes, _ := json.Marshal(models.WithdrawRequest{
		WalletID: walletID,
		Ticker:   ticker,
		Amount:   amount,
	})

	return bytes
}

func newBalance(walletID int) []byte {
	bytes, _ := json.Marshal(models.GetBalanceRequest{
		WalletID: walletID,
	})

	return bytes
}

func runProducer(cfg *broker.Config) {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%s/", cfg.Username, cfg.Password, cfg.Host, cfg.Port))
	// conn, err := amqp.Dial("amqp://app_rabbit:app_rabbit_password@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

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

	ctx := context.Background()
	log.Print("Begin producing messages")
	n := 4
	invoiceIDs := set.New[string](n)
	for i := 0; i < n; i++ {
		id := uuid.NewString()
		invoiceIDs.Insert(id)
		err = ch.PublishWithContext(ctx,
			"queries",                // exchange
			string(broker.OpInvoice), // routing key
			false,                    // mandatory
			false,                    // immediate
			amqp.Publishing{
				ContentType:   "application/json",
				CorrelationId: id,
				ReplyTo:       q.Name,
				Body:          newInvoice(1, "USD", 1),
			})
		failOnError(err, "Failed to publish a message")
	}

	log.Print("Wait for invoice's responses")
	for d := range msgs {
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
	err = ch.PublishWithContext(ctx,
		"queries",                 // exchange
		string(broker.OpWithdraw), // routing key
		false,                     // mandatory
		false,                     // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: withDrawID,
			ReplyTo:       q.Name,
			Body:          newWithdraw(1, "USD", 50),
		})
	failOnError(err, "Failed to publish a message")

	log.Print("Wait for withdraw response")
	for d := range msgs {
		if d.CorrelationId == withDrawID {
			log.Printf("Message: %s", d.Body)
			break
		}
	}

	log.Print("Produce balance request")
	balanceID := uuid.NewString()
	err = ch.PublishWithContext(ctx,
		"queries",                   // exchange
		string(broker.OpGetBalance), // routing key
		false,                       // mandatory
		false,                       // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: balanceID,
			ReplyTo:       q.Name,
			Body:          newBalance(1),
		})
	failOnError(err, "Failed to publish a message")

	log.Print("Wait for balance response")
	for d := range msgs {
		if d.CorrelationId == balanceID {
			log.Printf("Message: %s", d.Body)
			break
		}
	}
	log.Print("All responses received")
}

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Can't load config: %v", err)
	}

	// создаем подключение к базе данных
	dbCfg := &repository.Config{
		Host:     os.Getenv("DB_HOST"),
		Port:     os.Getenv("DB_PORT"),
		Username: os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		DBName:   os.Getenv("DB_NAME"),
		SSLMode:  os.Getenv("SSL_MODE"),
	}
	postgresRepo, err := repository.NewPostgresRepo(dbCfg)
	if err != nil {
		log.Fatalf("Can't connect to db: %v", err)
	}
	log.Print("Connected to db")

	// создаём подключение к брокеру сообщений
	brokerCfg := &broker.Config{
		Port:     os.Getenv("BR_PORT"),
		Host:     os.Getenv("BR_HOST"),
		Username: os.Getenv("BR_USER"),
		Password: os.Getenv("BR_PASSWORD"),
	}
	rabbit, err := broker.NewRabbitMQ(brokerCfg)
	if err != nil {
		log.Fatalf("Can't connect to message broker: %v", err)
	}
	log.Print("Connected to message broker")

	// создаём приложение
	ctx := context.Background()
	server := app.NewApp(postgresRepo, rabbit)
	server.RunConsumer(ctx)
	log.Print("App started, consume messages")

	// if err := repository.FillTestData(server.Repo); err != nil {
	// 	log.Fatalf("Cant't fill test data: %v", err)
	// }

	runProducer(brokerCfg)
	time.Sleep(30 * time.Minute)
	if err := server.Close(); err != nil {
		log.Fatalf("Can't stop app: %v", err)
	}
	log.Print("Stop app")
}
