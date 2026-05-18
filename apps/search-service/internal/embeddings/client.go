package embeddings

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	DefaultModel      = string(openai.EmbeddingModelTextEmbedding3Small)
	DefaultDimensions = int64(256)
)

type Client struct {
	inner      openai.Client
	model      openai.EmbeddingModel
	dimensions int64
}

func New(apiKey, model string, dimensions int64) *Client {
	if model == "" {
		model = DefaultModel
	}
	if dimensions == 0 {
		dimensions = DefaultDimensions
	}
	return &Client{
		inner:      openai.NewClient(option.WithAPIKey(apiKey)),
		model:      openai.EmbeddingModel(model),
		dimensions: dimensions,
	}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.inner.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model:      c.model,
		Input:      openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: []string{text}},
		Dimensions: openai.Int(c.dimensions),
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embeddings: empty response from API")
	}
	raw := resp.Data[0].Embedding
	vec := make([]float32, len(raw))
	for i, v := range raw {
		vec[i] = float32(v)
	}
	return vec, nil
}
