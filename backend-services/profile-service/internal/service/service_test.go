package service

import (
	"context"
	"errors"
	"profile-service/internal/domain"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateProfile_Success(t *testing.T) {
	repo := &mockRepo{
		CreateProfileMock: func(ctx context.Context, profReq *domain.CreateProfileRequest, id string) (*domain.Profile, error) {
			return &domain.Profile{ID: id, Username: profReq.Username}, nil
		},
	}

	sent := false
	producer := &mockProducer{
		SendMock: func(topic, key string, value []byte) error {
			sent = true
			return nil
		},
	}

	svc := NewService(repo, producer)
	prof, err := svc.CreateProfile(context.Background(), &domain.CreateProfileRequest{Username: "testuser"}, "id123")

	assert.NoError(t, err)
	assert.Equal(t, "testuser", prof.Username)
	assert.True(t, sent, "expected kafka event to be called")
}

func TestCreateProfile_RepoError(t *testing.T) {
	repo := &mockRepo{
		CreateProfileMock: func(ctx context.Context, profReq *domain.CreateProfileRequest, id string) (*domain.Profile, error) {
			return nil, errors.New("some error")
		},
	}

	sent := false
	producer := &mockProducer{
		SendMock: func(topic, key string, value []byte) error {
			sent = true
			return nil
		},
	}

	svc := NewService(repo, producer)
	_, err := svc.CreateProfile(context.Background(), &domain.CreateProfileRequest{Username: "testuser"}, "id123")
	assert.Error(t, err)
	assert.False(t, sent, "expected kafka event not to be called")
}

func TestGetProfileSuccess(t *testing.T) {
	repo := &mockRepo{
		GetProfileMock: func(ctx context.Context, id string) (*domain.Profile, error) {
			return &domain.Profile{Username: "testuser", ID: id}, nil
		},
	}

	svc := NewService(repo, &mockProducer{})
	prof, err := svc.GetProfile(context.Background(), "id123")

	assert.NoError(t, err)
	assert.Equal(t, "id123", prof.ID)
	assert.Equal(t, "testuser", prof.Username)
}

func TestGetProfileError(t *testing.T) {
	repo := &mockRepo{
		GetProfileMock: func(ctx context.Context, id string) (*domain.Profile, error) {
			return nil, domain.ErrProfileNotFound
		},
	}

	svc := NewService(repo, &mockProducer{})
	_, err := svc.GetProfile(context.Background(), "missing")

	assert.ErrorIs(t, err, domain.ErrProfileNotFound)
}
