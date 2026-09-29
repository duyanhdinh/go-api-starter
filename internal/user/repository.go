package user

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	database *sql.DB
}

func NewPostgresRepository(database *sql.DB) *PostgresRepository {
	return &PostgresRepository{database: database}
}

func repositoryError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == "users_email_unique" {
		return ErrEmailTaken
	}
	return err
}

func (repository *PostgresRepository) Create(ctx context.Context, input Input) (User, error) {
	var result User
	err := repository.database.QueryRowContext(ctx, `INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id, email, name`, input.Email, input.Name).Scan(&result.ID, &result.Email, &result.Name)
	return result, repositoryError(err)
}

func (repository *PostgresRepository) Get(ctx context.Context, id int64) (User, error) {
	var result User
	err := repository.database.QueryRowContext(ctx, `SELECT id, email, name FROM users WHERE id = $1`, id).Scan(&result.ID, &result.Email, &result.Name)
	return result, repositoryError(err)
}

func (repository *PostgresRepository) List(ctx context.Context, limit, offset int) ([]User, error) {
	rows, err := repository.database.QueryContext(ctx, `SELECT id, email, name FROM users ORDER BY id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]User, 0)
	for rows.Next() {
		var record User
		if err := rows.Scan(&record.ID, &record.Email, &record.Name); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) Update(ctx context.Context, id int64, input Input) (User, error) {
	var result User
	err := repository.database.QueryRowContext(ctx, `UPDATE users SET email = $1, name = $2 WHERE id = $3 RETURNING id, email, name`, input.Email, input.Name, id).Scan(&result.ID, &result.Email, &result.Name)
	return result, repositoryError(err)
}

func (repository *PostgresRepository) Delete(ctx context.Context, id int64) error {
	result, err := repository.database.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
