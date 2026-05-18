package server

import (
	"time"

	db "github.com/twitter/user-service/db/sqlc"
	"github.com/twitter/user-service/internal/user"
)

type healthzResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type provisionRequest struct {
	KeycloakSub string `json:"keycloak_sub" binding:"required"`
	Email       string `json:"email"        binding:"required,email"`
	DisplayName string `json:"display_name"`
}

type updateProfileRequest struct {
	Username       *string `json:"username"         binding:"omitempty,min=3,max=30"`
	DisplayName    *string `json:"display_name"     binding:"omitempty,max=50"`
	AvatarURL      *string `json:"avatar_url"       binding:"omitempty,max=500"`
	HeaderImageURL *string `json:"header_image_url" binding:"omitempty,max=500"`
	Bio            *string `json:"bio"              binding:"omitempty,max=160"`
	WebsiteURL     *string `json:"website_url"      binding:"omitempty,max=200"`
	Location       *string `json:"location"         binding:"omitempty,max=30"`
}

type userResponse struct {
	ID             string    `json:"id"`
	Email          *string   `json:"email"` // non-null only when viewer is the owner
	Username       *string   `json:"username"`
	DisplayName    *string   `json:"display_name"`
	AvatarURL      *string   `json:"avatar_url"`
	HeaderImageURL *string   `json:"header_image_url"`
	Bio            *string   `json:"bio"`
	WebsiteURL     *string   `json:"website_url"`
	Location       *string   `json:"location"`
	FollowerCount  int32     `json:"follower_count"`
	FollowingCount int32     `json:"following_count"`
	IsFollowing    bool      `json:"is_following"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type followResponse struct {
	Following bool `json:"following"`
}

type userListItem struct {
	ID            string  `json:"id"`
	Username      *string `json:"username"`
	DisplayName   *string `json:"display_name"`
	AvatarURL     *string `json:"avatar_url"`
	Bio           *string `json:"bio"`
	FollowerCount int32   `json:"follower_count"`
	IsFollowing   bool    `json:"is_following"`
}

type userListResponse struct {
	Users      []userListItem `json:"users"`
	NextCursor *string        `json:"next_cursor"`
}

func newUserListItem(u user.UserListItem) userListItem {
	return userListItem{
		ID:            u.ID,
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		AvatarURL:     u.AvatarUrl,
		Bio:           u.Bio,
		FollowerCount: u.FollowerCount,
		IsFollowing:   u.IsFollowing,
	}
}

func newUserResponseFromProfile(p *user.Profile, viewerID string) userResponse {
	r := userResponse{
		ID:             p.ID,
		Username:       p.Username,
		DisplayName:    p.DisplayName,
		AvatarURL:      p.AvatarUrl,
		HeaderImageURL: p.HeaderImageUrl,
		Bio:            p.Bio,
		WebsiteURL:     p.WebsiteUrl,
		Location:       p.Location,
		FollowerCount:  p.FollowerCount,
		FollowingCount: p.FollowingCount,
		IsFollowing:    p.IsFollowing,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
	if viewerID == p.ID {
		r.Email = &p.Email
	}
	return r
}

func newUserResponse(u db.User) userResponse {
	return userResponse{
		ID:             u.ID,
		Email:          &u.Email,
		Username:       u.Username,
		DisplayName:    u.DisplayName,
		AvatarURL:      u.AvatarUrl,
		HeaderImageURL: u.HeaderImageUrl,
		Bio:            u.Bio,
		WebsiteURL:     u.WebsiteUrl,
		Location:       u.Location,
		FollowerCount:  u.FollowerCount,
		FollowingCount: u.FollowingCount,
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
	}
}
