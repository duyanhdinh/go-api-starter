package user

import "errors"

var (
	ErrInvalid    = errors.New("invalid user input")
	ErrNotFound   = errors.New("user not found")
	ErrEmailTaken = errors.New("email already exists")
)

type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Input struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}
