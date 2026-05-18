package server

import (
	"time"

	db "github.com/twitter/tweet-service/db/sqlc"
	"github.com/twitter/shared/ptr"
	"github.com/twitter/tweet-service/internal/hashtag"
	"github.com/twitter/tweet-service/internal/tweet"
)

type healthzResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type tweetBodyRequest struct {
	Body     string  `json:"body"      binding:"max=280"`
	MediaID  *string `json:"media_id"  binding:"omitempty,max=50"`
	MediaURL *string `json:"media_url" binding:"omitempty,max=500"`
}

type authorResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type tweetResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Author       authorResponse `json:"author"`
	Body         string         `json:"body"`
	ReplyToID    *string        `json:"reply_to_id"`
	MediaID      *string        `json:"media_id"`
	MediaURL     *string        `json:"media_url"`
	LikeCount    int32          `json:"like_count"`
	RetweetCount int32          `json:"retweet_count"`
	ReplyCount   int32          `json:"reply_count"`
	Hashtags     []string       `json:"hashtags"`
	IsLiked      bool           `json:"is_liked"`
	IsRetweeted  bool           `json:"is_retweeted"`
	CreatedAt    time.Time      `json:"created_at"`
}

type retweetWrapper struct {
	ID        string         `json:"id"`
	Retweeter authorResponse `json:"retweeter"`
	CreatedAt time.Time      `json:"created_at"`
}

type feedItem struct {
	Kind    string          `json:"kind"`
	Tweet   tweetResponse   `json:"tweet"`
	Retweet *retweetWrapper `json:"retweet"`
}

type feedResponse struct {
	Items      []feedItem `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type likeResponse struct {
	Liked bool `json:"liked"`
}

type retweetResponse struct {
	Retweeted bool `json:"retweeted"`
}

type tweetsResponse struct {
	Tweets     []tweetResponse `json:"tweets"`
	NextCursor *string         `json:"next_cursor"`
}

type tweetResponseParams struct {
	id           string
	author       tweet.AuthorInfo
	body         string
	replyToID    *string
	mediaID      *string
	mediaURL     *string
	likeCount    int32
	retweetCount int32
	replyCount   int32
	createdAt    time.Time
	tags         []string
	isLiked      bool
	isRetweeted  bool
}

func tweetRowResponse(t db.GetTweetByIDRow, authors map[string]tweet.AuthorInfo, tags []string) tweetResponse {
	return newTweetResponse(tweetResponseParams{
		id:           t.ID,
		author:       authors[t.AuthorID],
		body:         t.Body,
		replyToID:    t.ReplyToID,
		mediaID:      t.MediaID,
		mediaURL:     t.MediaUrl,
		likeCount:    t.LikeCount,
		retweetCount: t.RetweetCount,
		replyCount:   t.ReplyCount,
		createdAt:    t.CreatedAt,
		tags:         tags,
		isLiked:      t.IsLiked,
		isRetweeted:  t.IsRetweeted,
	})
}

func newTweetResponse(p tweetResponseParams) tweetResponse {
	if p.tags == nil {
		p.tags = hashtag.Extract(p.body)
	}
	tweetType := "tweet"
	if p.replyToID != nil {
		tweetType = "reply"
	}
	return tweetResponse{
		ID:   p.id,
		Type: tweetType,
		Author: authorResponse{
			ID:          p.author.ID,
			Username:    p.author.Username,
			DisplayName: p.author.DisplayName,
			AvatarURL:   ptr.NonEmpty(p.author.AvatarURL),
		},
		Body:         p.body,
		ReplyToID:    p.replyToID,
		MediaID:      p.mediaID,
		MediaURL:     p.mediaURL,
		LikeCount:    p.likeCount,
		RetweetCount: p.retweetCount,
		ReplyCount:   p.replyCount,
		Hashtags:     p.tags,
		IsLiked:      p.isLiked,
		IsRetweeted:  p.isRetweeted,
		CreatedAt:    p.createdAt,
	}
}

func authorInfoToResponse(a tweet.AuthorInfo) authorResponse {
	return authorResponse{
		ID:          a.ID,
		Username:    a.Username,
		DisplayName: a.DisplayName,
		AvatarURL:   ptr.NonEmpty(a.AvatarURL),
	}
}
