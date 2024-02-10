package main

import (
	"bwg_transactional_system/internal/app"
	"bwg_transactional_system/internal/broker"
	"bwg_transactional_system/internal/helpers"
	"bwg_transactional_system/internal/repository"
	"context"
	"errors"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
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

	transactionalApp := app.NewApp(postgresRepo, rabbit)
	// запускаем 10 обработчиков сообщений
	for i := 0; i < 10; i++ {
		transactionalApp.RunConsumer(ctx)
	}
	log.Print("App started, consume messages")

	serverAddr := ":" + os.Getenv("SERVER_PORT")
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections.")
	}()

	// if err := helpers.FillTestData(transactionalApp.Repo); err != nil {
	// 	log.Fatalf("Can't fill test data: %v", err)
	// }

	time.Sleep(time.Second)
	helpers.RunTest2(brokerCfg) // только для тестирования

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan // wait ctrl + c

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}

	if err := transactionalApp.Close(); err != nil {
		log.Fatalf("Can't stop app: %v", err)
	}
	log.Println("Graceful shutdown complete.")
}
