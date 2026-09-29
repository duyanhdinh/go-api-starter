package user

import (
	"context"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const (
	MaxEmailBytes   = 254
	MaxNameRunes    = 100
	DefaultPageSize = 20
	MaxPageSize     = 100
)

type Repository interface {
	Create(context.Context, Input) (User, error)
	Get(context.Context, int64) (User, error)
	List(context.Context, int, int) ([]User, error)
	Update(context.Context, int64, Input) (User, error)
	Delete(context.Context, int64) error
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func normalize(input Input) (Input, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > MaxEmailBytes || strings.ContainsAny(input.Email, "\x00\r\n") {
		return Input{}, ErrInvalid
	}
	if !utf8.ValidString(input.Name) || input.Name == "" || utf8.RuneCountInString(input.Name) > MaxNameRunes || strings.ContainsRune(input.Name, '\x00') {
		return Input{}, ErrInvalid
	}
	return input, nil
}

func (service *Service) Create(ctx context.Context, input Input) (User, error) {
	input, err := normalize(input)
	if err != nil {
		return User{}, err
	}
	return service.repository.Create(ctx, input)
}

func (service *Service) Get(ctx context.Context, id int64) (User, error) {
	if id <= 0 {
		return User{}, ErrInvalid
	}
	return service.repository.Get(ctx, id)
}

func (service *Service) List(ctx context.Context, limit, offset int) ([]User, error) {
	if limit < 1 || limit > MaxPageSize || offset < 0 {
		return nil, ErrInvalid
	}
	return service.repository.List(ctx, limit, offset)
}

func (service *Service) Update(ctx context.Context, id int64, input Input) (User, error) {
	if id <= 0 {
		return User{}, ErrInvalid
	}
	input, err := normalize(input)
	if err != nil {
		return User{}, err
	}
	return service.repository.Update(ctx, id, input)
}

func (service *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalid
	}
	return service.repository.Delete(ctx, id)
}
