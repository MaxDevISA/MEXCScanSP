package types

// SpreadData представляет данные о спреде
type SpreadData struct {
	Symbol        string  `json:"symbol"`
	BestBid       float64 `json:"best_bid"`
	BestAsk       float64 `json:"best_ask"`
	SpreadPercent float64 `json:"spread_percent"`
	AbsoluteDiff  float64 `json:"absolute_diff"`
}
