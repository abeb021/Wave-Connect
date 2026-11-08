package repository

import (
	"context"
	"os"
	"profile-service/internal/domain"
	"testing"
)

// testDBURL — адрес БД для интеграционных тестов. Без него тесты скипаются,
// чтобы `go test ./...` проходил локально без Postgres.
//
//	TEST_DB_URL=postgres://postgres:postgres@localhost:5432/profile_db?sslmode=disable go test ./...
func testDBURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DB_URL")
	if url == "" {
		t.Skip("TEST_DB_URL не задан — интеграционный тест пропущен")
	}
	return url
}

func TestNewPool(t *testing.T) {
	ctx := context.Background()

	pool, err := NewPool(ctx, testDBURL(t))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping: %v", err)
	}
}

func TestCreateProfile(t *testing.T) {
	ctx := context.Background()

	pool, err := NewPool(ctx, testDBURL(t))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	repo := NewRepository(pool)

	profReq := &domain.CreateProfileRequest{
		Username: "testuser",
	}

	id := "00000000-0000-0000-0000-000000000001"

	// Тест идемпотентен: username уникален, поэтому чистим прошлую запись.
	if _, err := pool.Exec(ctx, `DELETE FROM profiles WHERE id = $1 OR username = $2`, id, profReq.Username); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	prof, err := repo.CreateProfile(ctx, profReq, id)
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	if prof.ID != id {
		t.Errorf("expected ID %s, got %s", id, prof.ID)
	}
	if prof.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", prof.Username)
	}
}
