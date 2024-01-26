package repository

import (
	"bwg_transactional_system/internal/models"
	"context"
	"fmt"
)

type Repository interface {
	CreateWallet(ctx context.Context) (*models.Wallet, error)
	CreateTicker(ctx context.Context, ticker string) error
	Invoice(ctx context.Context, req *models.InvoiceRequest) error
	WithDraw(ctx context.Context, req *models.WithdrawRequest) error
	GetBalance(ctx context.Context, req *models.GetBalanceRequest) (*models.GetBalanceResponse, error)
	Close() error
}

type LogicErrors error

func WalletDoesntExist(walletID int) LogicErrors {
	return fmt.Errorf("the requested wallet with id = %d, doesn't exist", walletID)
}

func TickerDoesntExist(ticker string) LogicErrors {
	return fmt.Errorf("the requested ticker with name = %s, doesn't exist", ticker)
}

func NotEnoughCoins(walletID int, ticker string) LogicErrors {
	return fmt.Errorf("there are not enough %s's on the wallet with id = %d to be debited", ticker, walletID)
}
