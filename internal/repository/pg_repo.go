package repository

import (
	"bwg_transactional_system/internal/models"
	"context"
	"database/sql"
	"fmt"
)

type PostgresRepo struct {
	db *sql.DB
}

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	DBName   string
	SSLMode  string
}

const createTableQuery = `
CREATE TABLE IF NOT EXISTS wallets
(
    wallet_id serial primary key
);

CREATE TABLE IF NOT EXISTS tickers
(
    ticker_id   serial primary key,
    name varchar(255) NOT NULL
);

CREATE TABLE IF NOT EXISTS balances
(
    wallet_id integer references wallets (wallet_id),
    ticker_id integer references tickers (ticker_id),
    amount    double precision CHECK (amount > 0) NOT NULL,
    PRIMARY KEY (wallet_id, ticker_id)
);

CREATE TABLE IF NOT EXISTS transactions
(
    id        serial primary key,
    wallet_id integer references wallets (wallet_id),
    ticker_id integer references tickers (ticker_id),
    amount    double precision NOT NULL,
    status    integer NOT NULL,
    CONSTRAINT valid_status CHECK (0 <= status AND status <= 2)
);`

func NewPostgresRepo(cfg *Config) (*PostgresRepo, error) {
	// postgresql://<username>:<password>@<hostname>:<port>/<dbname>
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.DBName, cfg.Password, cfg.SSLMode))
	if err != nil {
		return nil, err
	}

	// driver, err := postgres.WithInstance(db, &postgres.Config{})
	// if err != nil {
	// 	return nil, err
	// }
	// m, err := migrate.NewWithDatabaseInstance(
	// 	"file://migrations",
	// 	cfg.DBName, driver)
	// if err != nil {
	// 	return nil, err
	// }

	// err = m.Up()
	// if err != nil {
	// 	return nil, err
	// }

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if _, err := db.Exec(createTableQuery); err != nil {
		return nil, fmt.Errorf("falied to create tables: %v", err)
	}

	return &PostgresRepo{db: db}, nil
}

func (p *PostgresRepo) CreateWallet(ctx context.Context) (*models.Wallet, error) {
	var walletID int
	if err := p.db.QueryRowContext(ctx,
		"INSERT INTO wallets DEFAULT VALUES RETURNING wallet_id").Scan(&walletID); err != nil {
		return nil, err
	}

	return &models.Wallet{WalletID: walletID}, nil
}

func (p *PostgresRepo) CreateTicker(ctx context.Context, ticker string) error {
	if err := p.db.QueryRowContext(ctx,
		"INSERT INTO tickers (ticker_id, name) VALUES (default, $1)", ticker).Err(); err != nil {
		return err
	}

	return nil
}

func rollbackTx(tx *sql.Tx, queryError error) error {
	if err := tx.Rollback(); err != nil {
		return fmt.Errorf("transaction rollback error: %v, query error: %v", err, queryError)
	}

	return queryError
}

func (p *PostgresRepo) createTransaction(ctx context.Context, tx *sql.Tx, transaction *models.Transaction) error {
	// создаём запись в таблице transactions
	if err := tx.QueryRowContext(ctx,
		"INSERT INTO transactions (id, wallet_id, ticker_id, amount, status) VALUES (default, $1, $2, $3, $4) RETURNING id",
		transaction.WalletID, transaction.TickerID, transaction.Amount, int(transaction.Status)).Scan(&transaction.ID); err != nil {
		return err
	}

	return nil
}

func (p *PostgresRepo) updateTransactionStatus(ctx context.Context, tx *sql.Tx, transaction *models.Transaction) error {
	// меняем запись в таблице transactions
	if _, err := tx.ExecContext(ctx,
		"UPDATE transactions SET status = $1 where id = $2",
		int(transaction.Status), transaction.ID); err != nil {
		return err
	}

	return nil
}

func (p *PostgresRepo) checkWalletAndTicker(ctx context.Context, walletID int, ticker string) (int, error) {
	readTx, err := p.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}

	// проверяем то что нужный кошелёк существует
	var wID int
	if err := readTx.QueryRowContext(ctx,
		"SELECT wallet_id FROM wallets WHERE wallet_id = $1", walletID).Scan(&wID); err != nil {
		if err == sql.ErrNoRows {
			return 0, rollbackTx(readTx, WalletDoesntExist(walletID))
		}

		return 0, rollbackTx(readTx, err)
	}

	// проверяем что тикер существует
	var tickerID int
	if err := readTx.QueryRowContext(ctx,
		"SELECT ticker_id FROM tickers WHERE name = $1", ticker).Scan(&tickerID); err != nil {
		if err == sql.ErrNoRows {
			return 0, rollbackTx(readTx, TickerDoesntExist(ticker))
		}

		return 0, rollbackTx(readTx, err)
	}

	if err := readTx.Commit(); err != nil {
		return 0, err
	}

	return tickerID, nil
}

/*
1) Проверяем что существует кошелёк и тикер из операции, если нет, то сразу возвращаем ошибку

2) Открываем транзакцию

3) Создаём запись в таблице транзакций со статусом models.TransactionStatusCreated

4) Изменяем баланс на кошельке по нужному тикеру

5) Меняем статус транзакции на успешный

6) Подтверждаем транзакцию

В данной реализации никогда не возникнет ситуации с записью в таблице транзакций со статусом отличным от models.TransactionStatusSuccess.
Потому что все записи делаются в рамках одной транзакции.
*/
func (p *PostgresRepo) Invoice(ctx context.Context, req *models.InvoiceRequest) error {
	tickerID, err := p.checkWalletAndTicker(ctx, req.WalletID, req.Ticker)
	if err != nil {
		return err
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// создаём запись в таблице transactions
	transaction := &models.Transaction{
		WalletID: req.WalletID,
		TickerID: tickerID,
		Amount:   req.Amount,
		Status:   models.TransactionStatusCreated,
	}
	if err := p.createTransaction(ctx, tx, transaction); err != nil {
		return rollbackTx(tx, err)
	}

	// добавляем запись в таблицу balance
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO balances (wallet_id, ticker_id, amount) VALUES ($1, $2, $3) ON CONFLICT (wallet_id, ticker_id) DO UPDATE SET amount = balances.amount + $3",
		req.WalletID, tickerID, req.Amount); err != nil {
		return rollbackTx(tx, err)
	}

	// меняем статус транзакции на успешный
	transaction.Status = models.TransactionStatusSuccess
	if err := p.updateTransactionStatus(ctx, tx, transaction); err != nil {
		return rollbackTx(tx, err)
	}

	if err := tx.Commit(); err != nil {
		return rollbackTx(tx, err)
	}

	return nil
}

/*
1) Проверяем что существует кошелёк и тикер из операции, если нет, то сразу возвращаем ошибку

2) Открываем транзакцию

 3. Проверяем баланс на кошельке по данному тикеру. И в случае если его недостаточно для списания, то отменяем транзакцию,
    создаём запись о неудачной транзакции по списанию средств и возвращаем ошибку

4) Если баланса хватает для списания, то создаём запись в таблице транзакций со статусом models.TransactionStatusCreated

4) Изменяем баланс на кошельке по нужному тикеру

5) Меняем статус транзакции на успешный

6) Подтверждаем транзакцию
*/
func (p *PostgresRepo) WithDraw(ctx context.Context, req *models.WithdrawRequest) error {
	tickerID, err := p.checkWalletAndTicker(ctx, req.WalletID, req.Ticker)
	if err != nil {
		return err
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// проверяем баланс на кошельке
	var balance float32
	if err := tx.QueryRowContext(ctx,
		"SELECT amount FROM balances WHERE wallet_id = $1 AND ticker_id = $2", req.WalletID, tickerID).Scan(&balance); err != nil {
		if err == sql.ErrNoRows {
			return rollbackTx(tx, NotEnoughCoins(req.WalletID, req.Ticker))
		}
		return rollbackTx(tx, err)
	}
	// случай когда на счету недостаточно денег
	if balance < req.Amount {
		queryError := NotEnoughCoins(req.WalletID, req.Ticker)
		// сначала отменяем транзакцию
		if err := tx.Rollback(); err != nil {
			return fmt.Errorf("transaction rollback error: %v, query error: %v", err, queryError)
		}

		// Теперь создаём запись о неуспешной транзакции
		if _, err := p.db.ExecContext(ctx,
			"INSERT INTO transactions (id, wallet_id, ticker_id, amount, status) VALUES (default, $1, $2, $3, $4)",
			req.WalletID, tickerID, -req.Amount, models.TransactionStatusError); err != nil {
			return err
		}

		// после чего возвращаем ошибку о причине неудавшейся транзакции
		return queryError
	}

	// создаём запись в таблице transactions
	transaction := &models.Transaction{
		WalletID: req.WalletID,
		TickerID: tickerID,
		Amount:   -req.Amount,
		Status:   models.TransactionStatusCreated,
	}
	if err := p.createTransaction(ctx, tx, transaction); err != nil {
		return rollbackTx(tx, err)
	}

	// устанавливаем новый баланс
	if _, err := tx.ExecContext(ctx,
		"UPDATE balances SET amount = amount - $1 WHERE wallet_id = $2 AND ticker_id = $3", req.Amount, req.WalletID, tickerID); err != nil {
		return rollbackTx(tx, err)
	}

	// меняем статус записи в таблице transactions
	transaction.Status = models.TransactionStatusSuccess
	if err := p.updateTransactionStatus(ctx, tx, transaction); err != nil {
		return rollbackTx(tx, err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (p *PostgresRepo) getTickerName(ctx context.Context, tickerID int) (string, error) {
	var ticker string
	if err := p.db.QueryRowContext(ctx, "SELECT name FROM tickers WHERE ticker_id = $1", tickerID).Scan(&ticker); err != nil {
		return "", err
	}

	return ticker, nil
}

/*
1) Открываем транзакцию на чтение

2) Проверяем то что нужный кошелёк существует

3) Получаем актуальный баланс кошелька из таблицы balances

4) Получаем список замороженных транзакцией из таблицы transactions (со статусом models.TransactionStatusCreated)

5) Завершаем транзакцию на чтение

6) Формируем вывод из ответа бд
*/
func (p *PostgresRepo) GetBalance(ctx context.Context, req *models.GetBalanceRequest) (*models.GetBalanceResponse, error) {
	readTx, err := p.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}

	// проверяем то что нужный кошелёк существует
	var wID int
	if err := readTx.QueryRowContext(ctx, "SELECT wallet_id FROM wallets WHERE wallet_id = $1", req.WalletID).Scan(&wID); err != nil {
		if err == sql.ErrNoRows {
			return nil, rollbackTx(readTx, WalletDoesntExist(req.WalletID))
		}

		return nil, rollbackTx(readTx, err)
	}

	// получаем актуальный баланс кошелька из таблицы balances
	balanceRows, err := readTx.QueryContext(ctx, "SELECT ticker_id, amount FROM balances WHERE wallet_id = $1", req.WalletID)
	if err != nil {
		return nil, rollbackTx(readTx, err)
	}
	defer balanceRows.Close()

	// получаем список замороженных транзакцией из таблицы transactions
	frozenRows, err := readTx.QueryContext(ctx,
		"SELECT ticker_id, amount FROM transactions WHERE wallet_id = $1 AND status = $2", req.WalletID, models.TransactionStatusCreated)
	if err != nil {
		return nil, rollbackTx(readTx, err)
	}
	defer frozenRows.Close()

	// чтение может занять много времени, поэтому завершаем транзакцию сразу, чтобы не замедлять работу
	if err := readTx.Commit(); err != nil {
		return nil, err
	}

	// Заполняем актуальный баланс
	actualBalance := make(map[string]float32)
	for balanceRows.Next() {
		var tickerID int
		var amount float32
		if err != balanceRows.Scan(&tickerID, &amount) {
			return nil, err
		}

		// Представим что тикеров много и мы не может сохранить их все в словарь в оперативной памяти чтобы сконвертировать ID в название
		if ticker, err := p.getTickerName(ctx, tickerID); err != nil {
			return nil, err
		} else {
			actualBalance[ticker] = amount
		}
	}

	if err := balanceRows.Err(); err != nil {
		return nil, fmt.Errorf("error encountered while iterating over balance rows: %s", err)
	}

	// Теперь заполняем замороженные средства
	frozenBalance := make(map[string]float32)
	for frozenRows.Next() {
		var tickerID int
		var amount float32
		if err != frozenRows.Scan(&tickerID, &amount) {
			return nil, err
		}

		// Представим что тикеров много и мы не может сохранить их все в словарь в оперативной памяти чтобы сконвертировать ID в название
		if ticker, err := p.getTickerName(ctx, tickerID); err != nil {
			return nil, err
		} else {
			// один и тот же тикер может встречаться в разных транзакциях, если он уже был, то надо добавить к нему учёт новой транзакции
			if v, ok := frozenBalance[ticker]; ok {
				frozenBalance[ticker] = v + amount
			} else {
				frozenBalance[ticker] = amount
			}
		}
	}

	if err := frozenRows.Err(); err != nil {
		return nil, fmt.Errorf("error encountered while iterating over transaction rows: %s", err)
	}

	return &models.GetBalanceResponse{
		ActualBalance: actualBalance,
		FrozenBalance: frozenBalance,
	}, nil
}

func (p *PostgresRepo) Close() error {
	return p.db.Close()
}
