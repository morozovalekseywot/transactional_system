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
    amount    double precision CHECK (amount >= 0) NOT NULL,
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
);