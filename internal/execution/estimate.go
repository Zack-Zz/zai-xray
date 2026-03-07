package execution

import "unicode/utf8"

// estimateTokens provides a cheap fallback when provider usage is unavailable.
// It intentionally favors deterministic and fast estimation over exact tokenizer parity.
func estimateTokens(prompt, output string) (promptTokens, completionTokens, total int) {
	promptTokens = estimateTextTokens(prompt)
	completionTokens = estimateTextTokens(output)
	total = promptTokens + completionTokens
	return
}

func estimateTokensFromBytes(requestBytes, responseBytes int64) (promptTokens, completionTokens, total int) {
	if requestBytes < 0 {
		requestBytes = 0
	}
	if responseBytes < 0 {
		responseBytes = 0
	}
	// Rough heuristic: ~4 bytes per token for mixed English/code text.
	promptTokens = int((requestBytes + 3) / 4)
	completionTokens = int((responseBytes + 3) / 4)
	total = promptTokens + completionTokens
	return
}

func estimateTextTokens(text string) int {
	runes := utf8.RuneCountInString(text)
	if runes == 0 {
		return 0
	}
	// 1 token ~= 4 chars for rough fallback, rounded up.
	return (runes + 3) / 4
}
