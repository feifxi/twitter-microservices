package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
)

const (
	IndexTweets = "tweets"
	IndexUsers  = "users"

	vectorDimension = 256
)

// like_count and retweet_count are snapshot values for ranking; read paths re-fetch live counts from Redis.
type TweetDoc struct {
	ID           string    `json:"id"`
	AuthorID     string    `json:"author_id"`
	Body         string    `json:"body"`
	Hashtags     []string  `json:"hashtags"`
	LikeCount    int32     `json:"like_count"`
	RetweetCount int32     `json:"retweet_count"`
	ReplyToID    string    `json:"reply_to_id,omitempty"`
	MediaID      string    `json:"media_id,omitempty"`
	MediaURL     string    `json:"media_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	Vector       []float32 `json:"vector,omitempty"`
}

// avatar_url is retrieval-only (keyword in the mapping) — not scored.
type UserDoc struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	AvatarURL   string    `json:"avatar_url"`
	UpdatedAt   time.Time `json:"updated_at"`
	Vector      []float32 `json:"vector,omitempty"`
}

type Client struct {
	os   *opensearchgo.Client
	addr string
}

func New(addr string) (*Client, error) {
	c, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{addr},
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Client{os: c, addr: addr}, nil
}

func (c *Client) Addr() string { return c.addr }

// Ping issues a HEAD against the root endpoint — cheapest reachability check.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := opensearchapi.PingRequest{}.Do(ctx, c.os)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("opensearch ping: %s", resp.Status())
	}
	return nil
}

func (c *Client) EnsureIndices(ctx context.Context) error {
	if err := c.createIndex(ctx, IndexTweets, tweetMapping()); err != nil {
		return fmt.Errorf("ensure tweets index: %w", err)
	}
	if err := c.createIndex(ctx, IndexUsers, userMapping()); err != nil {
		return fmt.Errorf("ensure users index: %w", err)
	}
	return nil
}

func (c *Client) IndexTweet(ctx context.Context, doc TweetDoc) error {
	return c.upsert(ctx, IndexTweets, doc.ID, doc)
}

func (c *Client) DeleteTweet(ctx context.Context, id string) error {
	return c.delete(ctx, IndexTweets, id)
}

func (c *Client) IndexUser(ctx context.Context, doc UserDoc) error {
	return c.upsert(ctx, IndexUsers, doc.ID, doc)
}

// vector nil falls back to keyword regardless of mode.
func (c *Client) SearchTweets(ctx context.Context, query string, vector []float32, mode string, from, size int) ([]TweetDoc, error) {
	body := buildTweetQuery(query, vector, mode, from, size)
	return c.searchTweets(ctx, body)
}

func (c *Client) SearchUsers(ctx context.Context, query string, from, size int) ([]UserDoc, error) {
	body := buildUserQuery(query, from, size)
	return c.searchUsers(ctx, body)
}

func (c *Client) createIndex(ctx context.Context, index, mapping string) error {
	exists, err := c.indexExists(ctx, index)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	req := opensearchapi.IndicesCreateRequest{
		Index: index,
		Body:  strings.NewReader(mapping),
	}
	resp, err := req.Do(ctx, c.os)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create index %s: %s", index, b)
	}
	return nil
}

func (c *Client) indexExists(ctx context.Context, index string) (bool, error) {
	req := opensearchapi.IndicesExistsRequest{Index: []string{index}}
	resp, err := req.Do(ctx, c.os)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func (c *Client) upsert(ctx context.Context, index, id string, doc any) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	req := opensearchapi.IndexRequest{
		Index:      index,
		DocumentID: id,
		Body:       bytes.NewReader(data),
	}
	resp, err := req.Do(ctx, c.os)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert %s/%s: %s", index, id, b)
	}
	return nil
}

func (c *Client) delete(ctx context.Context, index, id string) error {
	req := opensearchapi.DeleteRequest{Index: index, DocumentID: id}
	resp, err := req.Do(ctx, c.os)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete %s/%s: %s", index, id, b)
	}
	return nil
}

func (c *Client) searchTweets(ctx context.Context, body string) ([]TweetDoc, error) {
	req := opensearchapi.SearchRequest{
		Index: []string{IndexTweets},
		Body:  strings.NewReader(body),
	}
	resp, err := req.Do(ctx, c.os)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search tweets: %s", b)
	}

	var result struct {
		Hits struct {
			Hits []struct {
				Source TweetDoc `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	docs := make([]TweetDoc, len(result.Hits.Hits))
	for i, h := range result.Hits.Hits {
		docs[i] = h.Source
	}
	return docs, nil
}

func (c *Client) searchUsers(ctx context.Context, body string) ([]UserDoc, error) {
	req := opensearchapi.SearchRequest{
		Index: []string{IndexUsers},
		Body:  strings.NewReader(body),
	}
	resp, err := req.Do(ctx, c.os)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search users: %s", b)
	}

	var result struct {
		Hits struct {
			Hits []struct {
				Source UserDoc `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	docs := make([]UserDoc, len(result.Hits.Hits))
	for i, h := range result.Hits.Hits {
		docs[i] = h.Source
	}
	return docs, nil
}

func buildTweetQuery(query string, vector []float32, mode string, from, size int) string {
	// Single "#token" matches `hashtags` exactly; multi_match would also hit body text.
	if strings.HasPrefix(query, "#") && !strings.ContainsAny(query, " \t\n") {
		return fmt.Sprintf(`{
			"from": %d, "size": %d,
			"query": { "term": { "hashtags": %s } },
			"sort": [{"created_at": "desc"}]
		}`, from, size, jsonString(query))
	}

	// embedding fallback: keyword still works without OpenAI
	if vector == nil {
		mode = "keyword"
	}

	switch mode {
	case "semantic":
		return fmt.Sprintf(`{
			"from": %d, "size": %d,
			"query": {
				"knn": {
					"vector": { "vector": %s, "k": %d }
				}
			}
		}`, from, size, jsonFloats(vector), size)

	case "hybrid":
		return fmt.Sprintf(`{
			"from": %d, "size": %d,
			"query": {
				"bool": {
					"should": [
						{
							"multi_match": {
								"query": %s,
								"fields": ["body", "hashtags^2"],
								"boost": 0.5
							}
						},
						{
							"knn": {
								"vector": { "vector": %s, "k": %d, "boost": 0.5 }
							}
						}
					],
					"minimum_should_match": 1
				}
			}
		}`, from, size, jsonString(query), jsonFloats(vector), size)

	default:
		return fmt.Sprintf(`{
			"from": %d, "size": %d,
			"query": {
				"multi_match": {
					"query": %s,
					"fields": ["body", "hashtags^2"],
					"type": "best_fields"
				}
			},
			"sort": [{"_score": "desc"}, {"created_at": "desc"}]
		}`, from, size, jsonString(query))
	}
}

func buildUserQuery(query string, from, size int) string {
	return fmt.Sprintf(`{
		"from": %d, "size": %d,
		"query": {
			"multi_match": {
				"query": %s,
				"fields": ["username^3", "display_name^2", "bio"],
				"type": "best_fields"
			}
		}
	}`, from, size, jsonString(query))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonFloats(v []float32) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func tweetMapping() string {
	return fmt.Sprintf(`{
		"settings": {
			"knn": true
		},
		"mappings": {
			"properties": {
				"id":            { "type": "keyword" },
				"author_id":     { "type": "keyword" },
				"body":          { "type": "text", "analyzer": "english" },
				"hashtags":      { "type": "keyword" },
				"like_count":    { "type": "integer" },
				"retweet_count": { "type": "integer" },
				"reply_to_id":   { "type": "keyword", "index": false },
				"media_id":      { "type": "keyword", "index": false },
				"media_url":     { "type": "keyword", "index": false },
				"created_at":    { "type": "date" },
				"vector": {
					"type": "knn_vector",
					"dimension": %d,
					"method": {
						"name": "hnsw",
						"space_type": "cosinesimil",
						"engine": "lucene",
						"parameters": { "ef_construction": 128, "m": 16 }
					}
				}
			}
		}
	}`, vectorDimension)
}

func userMapping() string {
	return fmt.Sprintf(`{
		"settings": {
			"knn": true
		},
		"mappings": {
			"properties": {
				"id":           { "type": "keyword" },
				"username":     { "type": "text", "analyzer": "standard", "fields": { "keyword": { "type": "keyword" } } },
				"display_name": { "type": "text", "analyzer": "standard" },
				"bio":          { "type": "text", "analyzer": "english" },
				"avatar_url":   { "type": "keyword", "index": false },
				"updated_at":   { "type": "date" },
				"vector": {
					"type": "knn_vector",
					"dimension": %d,
					"method": {
						"name": "hnsw",
						"space_type": "cosinesimil",
						"engine": "lucene",
						"parameters": { "ef_construction": 128, "m": 16 }
					}
				}
			}
		}
	}`, vectorDimension)
}
