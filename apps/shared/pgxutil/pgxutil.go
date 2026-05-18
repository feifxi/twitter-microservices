package pgxutil

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/twitter/shared/httperr"
)

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// pgx.ErrNoRows → ErrNotFound, unique violation → ErrConflict, others pass through.
func MapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return httperr.ErrNotFound
	}
	if IsUniqueViolation(err) {
		return httperr.ErrConflict
	}
	return err
}
