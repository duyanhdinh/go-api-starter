package user

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
)

type memoryRepository struct {
	users   map[int64]User
	nextID  int64
	failure error
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{users: make(map[int64]User)}
}

func (repository *memoryRepository) Create(_ context.Context, input Input) (User, error) {
	if repository.failure != nil {
		return User{}, repository.failure
	}
	for _, existing := range repository.users {
		if existing.Email == input.Email {
			return User{}, ErrEmailTaken
		}
	}
	repository.nextID++
	result := User{ID: repository.nextID, Email: input.Email, Name: input.Name}
	repository.users[result.ID] = result
	return result, nil
}

func (repository *memoryRepository) Get(_ context.Context, id int64) (User, error) {
	if repository.failure != nil {
		return User{}, repository.failure
	}
	result, exists := repository.users[id]
	if !exists {
		return User{}, ErrNotFound
	}
	return result, nil
}

func (repository *memoryRepository) List(_ context.Context, limit, offset int) ([]User, error) {
	if repository.failure != nil {
		return nil, repository.failure
	}
	result := make([]User, 0)
	for _, record := range repository.users {
		result = append(result, record)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	if offset >= len(result) {
		return []User{}, nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (repository *memoryRepository) Update(ctx context.Context, id int64, input Input) (User, error) {
	if _, err := repository.Get(ctx, id); err != nil {
		return User{}, err
	}
	for _, existing := range repository.users {
		if existing.ID != id && existing.Email == input.Email {
			return User{}, ErrEmailTaken
		}
	}
	result := User{ID: id, Email: input.Email, Name: input.Name}
	repository.users[id] = result
	return result, nil
}

func (repository *memoryRepository) Delete(ctx context.Context, id int64) error {
	if _, err := repository.Get(ctx, id); err != nil {
		return err
	}
	delete(repository.users, id)
	return nil
}

func TestServiceValidationAndNormalization(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)
	ctx := context.Background()
	created, err := service.Create(ctx, Input{Email: " An@Example.COM ", Name: " Nguyễn An "})
	if err != nil || created.Email != "an@example.com" || created.Name != "Nguyễn An" {
		t.Fatalf("unexpected normalized user: %+v, %v", created, err)
	}
	for _, input := range []Input{
		{}, {Email: "invalid", Name: "An"}, {Email: "An <an@example.com>", Name: "An"},
		{Email: "an@example.com", Name: " "}, {Email: "an@example.com", Name: strings.Repeat("ế", 101)},
		{Email: "an@example.com", Name: "a\x00b"}, {Email: strings.Repeat("a", 250) + "@example.com", Name: "An"},
	} {
		if _, err := service.Create(ctx, input); !errors.Is(err, ErrInvalid) {
			t.Errorf("Create(%+v): %v", input, err)
		}
		if _, err := service.Update(ctx, created.ID, input); !errors.Is(err, ErrInvalid) {
			t.Errorf("Update(%+v): %v", input, err)
		}
	}
	if len(repository.users) != 1 || repository.users[created.ID] != created {
		t.Fatal("invalid input mutated storage")
	}
	for _, id := range []int64{0, -1} {
		if _, err := service.Get(ctx, id); err != ErrInvalid {
			t.Fatal("Get accepted invalid ID")
		}
		if _, err := service.Update(ctx, id, Input{}); err != ErrInvalid {
			t.Fatal("Update accepted invalid ID")
		}
		if err := service.Delete(ctx, id); err != ErrInvalid {
			t.Fatal("Delete accepted invalid ID")
		}
	}
	for _, pagination := range [][2]int{{0, 0}, {101, 0}, {20, -1}} {
		if _, err := service.List(ctx, pagination[0], pagination[1]); err != ErrInvalid {
			t.Fatal("accepted invalid pagination")
		}
	}
	repository.failure = errors.New("storage unavailable")
	if _, err := service.Get(ctx, created.ID); err != repository.failure {
		t.Fatal("storage error lost")
	}
}

func TestUserLimitBoundaries(t *testing.T) {
	for _, testCase := range []struct {
		input Input
		valid bool
	}{
		{Input{Email: strings.Repeat("a", 242) + "@example.com", Name: "An"}, true},
		{Input{Email: strings.Repeat("a", 243) + "@example.com", Name: "An"}, false},
		{Input{Email: "an@example.com", Name: strings.Repeat("ế", 100)}, true},
		{Input{Email: "an@example.com", Name: strings.Repeat("ế", 101)}, false},
	} {
		_, err := normalize(testCase.input)
		if (err == nil) != testCase.valid {
			t.Fatalf("email bytes=%d, name bytes=%d: valid=%v, err=%v", len(testCase.input.Email), len(testCase.input.Name), testCase.valid, err)
		}
	}
	service := NewService(newMemoryRepository())
	for _, limit := range []int{1, 100, 101} {
		_, err := service.List(context.Background(), limit, 0)
		if (err == nil) != (limit <= 100) {
			t.Fatalf("limit=%d: err=%v", limit, err)
		}
	}
}
