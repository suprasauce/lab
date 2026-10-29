package utils

import (
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/models"
)

// WriteBacktestCSV writes one row per trade, with monetary values in rupees and
// timestamps in IST. Open trades have blank exit fields and realized P&L.
// The caller owns the writer and any file creation or closing.
func WriteBacktestCSV(w io.Writer, result *engine.BacktestResult) error {
	if result == nil {
		return fmt.Errorf("backtest result cannot be nil")
	}
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return fmt.Errorf("load CSV timezone: %w", err)
	}
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{
		"expiry", "underlying", "strike", "option_type", "side", "entry_timestamp",
		"exit_timestamp", "entry_price", "exit_price", "quantity", "pnl",
	}); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}
	for _, cycle := range result.Executions {
		for _, leg := range cycle {
			if leg == nil {
				return fmt.Errorf("CSV result contains a nil leg")
			}
			if len(leg.Trades) == 0 {
				continue
			}
			if leg.Contract == nil {
				return fmt.Errorf("CSV leg with trades has no selected contract")
			}
			for _, trade := range leg.Trades {
				exitTime, exitPrice, pnl := "", "", ""
				if trade.Status == models.TradeClosed {
					exitTime = trade.ExitTime.In(location).Format(time.RFC3339)
					exitPrice = formatPrice(trade.ExitPrice)
					pnl = formatPrice(trade.PnL)
				}
				if err := writer.Write([]string{
					leg.Contract.Expiry.Format(time.DateOnly), result.Underlying,
					formatPrice(leg.Contract.StrikePrice), string(leg.Contract.OptionType), string(trade.Side),
					trade.EntryTime.In(location).Format(time.RFC3339), exitTime,
					formatPrice(trade.EntryPrice), exitPrice, strconv.FormatInt(trade.Quantity, 10), pnl,
				}); err != nil {
					return fmt.Errorf("write CSV trade: %w", err)
				}
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush CSV: %w", err)
	}
	return nil
}

func formatPrice(price models.Price) string {
	return big.NewRat(int64(price), 100).FloatString(2)
}
