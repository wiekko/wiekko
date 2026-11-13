package controller

import (
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
	"modernc.org/sqlite"
)

// internalErrorMessage is what a client is told when the failure is not
// theirs to fix.
const internalErrorMessage = "Something went wrong on the server. Please try again."

// isDatabaseError reports whether err comes from the database driver: a
// violated constraint, a syntax error, a lost connection. Service errors are
// mostly written for the person making the request and are passed on to them,
// but a driver error names tables, columns and constraints, which is of no
// use to them and none of their business.
func isDatabaseError(err error) bool {
	var pg *pgconn.PgError
	var lite *sqlite.Error
	var conn *pgconn.ConnectError
	return errors.As(err, &pg) || errors.As(err, &lite) || errors.As(err, &conn)
}

// publicError returns the message to send to the client for err: its own text,
// or, for a database error, a generic one after logging the real cause.
func publicError(err error) (message string, internal bool) {
	if isDatabaseError(err) {
		slog.Error("request failed with a database error", "err", err)
		return internalErrorMessage, true
	}
	return err.Error(), false
}

// errorText is publicError for callers that only need the message, such as a
// page fragment showing why an action failed.
func errorText(err error) string {
	message, _ := publicError(err)
	return message
}
