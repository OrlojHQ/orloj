package agentruntime

// Chunk represents one segment of a chunked document.
type Chunk struct {
	Index  int    `json:"index"`
	Text   string `json:"text"`
	Offset int    `json:"offset"`
}

// ChunkText splits text into overlapping windows of chunkSize characters.
// overlap controls how many characters from the end of the previous chunk
// are repeated at the start of the next. Zero or negative values are
// replaced with sensible defaults (1000 / 200).
func ChunkText(text string, chunkSize, overlap int) []Chunk {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if overlap < 0 {
		overlap = 200
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 5
	}

	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return nil
	}

	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}

	var chunks []Chunk
	for offset := 0; offset < n; offset += step {
		end := offset + chunkSize
		if end > n {
			end = n
		}
		chunks = append(chunks, Chunk{
			Index:  len(chunks),
			Text:   string(runes[offset:end]),
			Offset: offset,
		})
		if end == n {
			break
		}
	}
	return chunks
}
