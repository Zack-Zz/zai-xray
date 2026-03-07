package providers

type Price struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

var openAIPrices = map[string]Price{
	"gpt-4o":       {InputPerMTok: 2.50, OutputPerMTok: 10.00},
	"gpt-4o-mini":  {InputPerMTok: 0.15, OutputPerMTok: 0.60},
	"gpt-4.1":      {InputPerMTok: 2.00, OutputPerMTok: 8.00},
	"gpt-4.1-mini": {InputPerMTok: 0.40, OutputPerMTok: 1.60},
}

func estimateCostWithTable(table map[string]Price, model string, promptTokens, completionTokens int) (float64, bool) {
	price, ok := table[model]
	if !ok {
		return 0, false
	}
	inCost := (float64(promptTokens) / 1_000_000.0) * price.InputPerMTok
	outCost := (float64(completionTokens) / 1_000_000.0) * price.OutputPerMTok
	return inCost + outCost, true
}
