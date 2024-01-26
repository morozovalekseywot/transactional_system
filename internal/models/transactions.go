package models

import "errors"

var ValidationAmountError = errors.New("amount lower then 0")

type Wallet struct {
	WalletID int
}

type TransactionStatus int

const (
	TransactionStatusSuccess TransactionStatus = 0
	TransactionStatusError   TransactionStatus = 1
	TransactionStatusCreated TransactionStatus = 2
)

type Transaction struct {
	ID       int               `json:"id"`
	WalletID int               `json:"wallet_id"`
	TickerID int               `json:"ticker_id"`
	Amount   float32           `json:"amount"`
	Status   TransactionStatus `json:"status,omitempty"`
}

// - Invoice -> человеку зачисляются средства по ручке "/invoice" с такими параметрами в теле,
// как код валюты ("USD", "RUB", "EUR", etc.), количество средств (число с плавающей точкой), номер кошелька или карты.
type InvoiceRequest struct {
	WalletID int     `json:"wallet_id"`
	Ticker   string  `json:"ticker"`
	Amount   float32 `json:"amount"`
}

func (req *InvoiceRequest) Validate() error {
	if req.Amount <= 0 {
		return ValidationAmountError
	}

	return nil
}

// Withdraw -> человек выводит средства со своего баланса по валюте, с параметрами в теле
// код валюты, количество средств, номер кошелька или карты куда зачисляются средства.
type WithdrawRequest struct {
	WalletID int     `json:"wallet_id"`
	Ticker   string  `json:"ticker"`
	Amount   float32 `json:"amount"`
}

func (req *WithdrawRequest) Validate() error {
	if req.Amount <= 0 {
		return ValidationAmountError
	}

	return nil
}

// Должна быть реализована ручка по получению актуального и замороженного баланса клиентов.
// Актуальный баланс это тот баланс, который можно вывести. Замороженный баланс - баланс со статусом "Created".
type GetBalanceRequest struct {
	WalletID int `json:"wallet_id"`
}

type GetBalanceResponse struct {
	ActualBalance map[string]float32 `json:"actual_balance"`
	FrozenBalance map[string]float32 `json:"frozen_balance"`
}
