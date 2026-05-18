package server

import (
	"time"

	"github.com/twitter/search-service/internal/search"
)

type healthzResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type authorSnap struct {
	ID          string  `json:"id"`
	Username    *string `json:"username"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type tweetResult struct {
	ID           string      `json:"id"`
	Author       *authorSnap `json:"author"`
	Body         string      `json:"body"`
	Hashtags     []string    `json:"hashtags"`
	ReplyToID    *string     `json:"reply_to_id"`
	MediaID      *string     `json:"media_id"`
	MediaURL     *string     `json:"media_url"`
	LikeCount    int32       `json:"like_count"`
	RetweetCount int32       `json:"retweet_count"`
	ReplyCount   int32       `json:"reply_count"`
	IsLiked      bool        `json:"is_liked"`
	IsRetweeted  bool        `json:"is_retweeted"`
	CreatedAt    time.Time   `json:"created_at"`
}

type tweetSearchResponse struct {
	Tweets     []tweetResult `json:"tweets"`
	NextCursor *string       `json:"next_cursor"`
}

func newTweetResult(r search.TweetResult) tweetResult {
	hashtags := r.Doc.Hashtags
	if hashtags == nil {
		hashtags = []string{}
	}
	return tweetResult{
		ID:           r.Doc.ID,
		Author:       newAuthorSnap(r.Doc.AuthorID, r.Author),
		Body:         r.Doc.Body,
		Hashtags:     hashtags,
		ReplyToID:    nonEmptyPtr(r.Doc.ReplyToID),
		MediaID:      nonEmptyPtr(r.Doc.MediaID),
		MediaURL:     nonEmptyPtr(r.Doc.MediaURL),
		LikeCount:    r.Counts.LikeCount,
		RetweetCount: r.Counts.RetweetCount,
		ReplyCount:   r.Counts.ReplyCount,
		IsLiked:      r.IsLiked,
		IsRetweeted:  r.IsRetweeted,
		CreatedAt:    r.Doc.CreatedAt,
	}
}

func newAuthorSnap(authorID string, a search.AuthorSnapshot) *authorSnap {
	return &authorSnap{
		ID:          authorID,
		Username:    nonEmptyPtr(a.Username),
		DisplayName: nonEmptyPtr(a.DisplayName),
		AvatarURL:   nonEmptyPtr(a.AvatarURL),
	}
}

func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type userResult struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	AvatarURL     string `json:"avatar_url"`
	Bio           string `json:"bio"`
	FollowerCount int64  `json:"follower_count"`
	IsFollowing   bool   `json:"is_following"`
}

type userSearchResponse struct {
	Users      []userResult `json:"users"`
	NextCursor *string      `json:"next_cursor"`
}

func newUserResult(r search.UserResult) userResult {
	return userResult{
		ID:            r.Doc.ID,
		Username:      r.Doc.Username,
		DisplayName:   r.Doc.DisplayName,
		AvatarURL:     r.Doc.AvatarURL,
		Bio:           r.Doc.Bio,
		FollowerCount: r.FollowerCount,
		IsFollowing:   r.IsFollowing,
	}
}
