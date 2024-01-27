package main

import (
	"bwg_transactional_system/internal/app"
	"bwg_transactional_system/internal/broker"
	"bwg_transactional_system/internal/helpers"
	"bwg_transactional_system/internal/repository"
	"context"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"os"
)

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
	if err := postgresRepo.TruncateBalances(ctx); err != nil { // только для тестов
		log.Fatalf("Can't truncate balances: %v", err)
	}
	if err := postgresRepo.TruncateTransactions(ctx); err != nil { // только для тестов
		log.Fatalf("Can't truncate transactions: %v", err)
	}

	server := app.NewApp(postgresRepo, rabbit)
	// запускаем 4 обработчика сообщений
	for i := 0; i < 10; i++ {
		server.RunConsumer(ctx)
	}
	log.Print("App started, consume messages")

	// if err := helpers.FillTestData(server.Repo); err != nil {
	// 	log.Fatalf("Cant't fill test data: %v", err)
	// }

	helpers.RunTest2(brokerCfg) // только для тестирования
	time.Sleep(30 * time.Minute)
	if err := server.Close(); err != nil {
		log.Fatalf("Can't stop app: %v", err)
	}
	log.Print("Stop app")
}
