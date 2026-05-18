package events

import "time"

const (
	TopicTweetCreated     = "tweet.created"
	TopicTweetDeleted     = "tweet.deleted"
	TopicTweetLiked       = "tweet.liked"
	TopicTweetRetweeted   = "tweet.retweeted"
	TopicTweetUnretweeted = "tweet.unretweeted"
	TopicTweetReplied     = "tweet.replied"
	TopicUserCreated    = "user.created"
	TopicUserFollowed   = "user.followed"
	TopicUserUpdated    = "user.updated"
	TopicMediaUploaded  = "media.uploaded"

	// Dead-letter queue topics — one per consumer group.
	// Messages that cannot be deserialized (poison) are routed here
	// so the partition keeps moving and the original payload is preserved
	// for inspection and replay via cmd/dlq-replay.
	TopicNotificationDLQ = "notification-service-cg.dlq"
	TopicFeedDLQ         = "feed-service-cg.dlq"
	TopicSearchDLQ       = "search-service-cg.dlq"
)

type TweetCreatedEvent struct {
	TweetID   string    `json:"tweet_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	Hashtags  []string  `json:"hashtags"`
	ReplyToID string    `json:"reply_to_id,omitempty"`
	MediaID   string    `json:"media_id,omitempty"`
	MediaURL  string    `json:"media_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type TweetDeletedEvent struct {
	TweetID  string `json:"tweet_id"`
	AuthorID string `json:"author_id"`
}

type TweetLikedEvent struct {
	TweetID  string   `json:"tweet_id"`
	LikerID  string   `json:"liker_id"`
	AuthorID string   `json:"author_id"`
	Hashtags []string `json:"hashtags"`
}

type TweetRetweetedEvent struct {
	RetweetID        string    `json:"retweet_id"`
	OriginalTweetID  string    `json:"original_tweet_id"`
	OriginalAuthorID string    `json:"original_author_id"`
	RetweeterID      string    `json:"retweeter_id"`
	CreatedAt        time.Time `json:"created_at"`
}

type TweetUnretweetedEvent struct {
	RetweetID       string `json:"retweet_id"`
	OriginalTweetID string `json:"original_tweet_id"`
	RetweeterID     string `json:"retweeter_id"`
}

type TweetRepliedEvent struct {
	ReplyID        string `json:"reply_id"`
	ParentTweetID  string `json:"parent_tweet_id"`
	ParentAuthorID string `json:"parent_author_id"`
	ReplierID      string `json:"replier_id"`
}

type UserCreatedEvent struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserFollowedEvent struct {
	FollowerID string    `json:"follower_id"`
	FolloweeID string    `json:"followee_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type UserUpdatedEvent struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url"`
}

type MediaUploadedEvent struct {
	MediaID    string `json:"media_id"`
	UploaderID string `json:"uploader_id"`
	S3Key      string `json:"s3_key"`
}
