package helpers

import (
	"bwg_transactional_system/internal/repository"
	"context"
	"fmt"
)

func FillTestData(repo repository.Repository) error {
	ctx := context.Background()
	tickers := []string{"USD", "RUB", "EUR", "USDT"}
	for _, t := range tickers {
		err := repo.CreateTicker(ctx, t)
		if err != nil {
			return fmt.Errorf("can't create ticker %s: %v", t, err)
		}
	}
	for i := 1; i < 10; i++ {
		_, err := repo.CreateWallet(ctx)
		if err != nil {
			return fmt.Errorf("can't create wallet: %v", err)
		}
	}

	return nil
}
