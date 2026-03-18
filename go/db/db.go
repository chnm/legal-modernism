package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

// Connect returns a pool of connections to the database, which is concurrency
// safe. Uses the pgx interface.
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	timeout, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	connstr, err := getConnString()
	if err != nil {
		return nil, err
	}

	db, err := pgxpool.Connect(timeout, connstr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	err = db.Ping(timeout)
	if err != nil {
		return nil, fmt.Errorf("error pinging database: %w", err)
	}

	return db, nil
}

// Host returns the database host from the LAW_DBSTR connection string,
// or "unknown" if the string is absent or cannot be parsed.
func Host() string {
	connstr, err := getConnString()
	if err != nil {
		return "unknown"
	}
	config, err := pgxpool.ParseConfig(connstr)
	if err != nil {
		return "unknown"
	}
	return config.ConnConfig.Host
}

// getConnString returns the DB connection string. It first checks for
// LAW_DBSTR. If that is not set, it assembles a connection string from
// individual LAW_DB_* environment variables.
func getConnString() (string, error) {
	connstr, exists := os.LookupEnv("LAW_DBSTR")
	if exists {
		return connstr, nil
	}
	return buildConnString()
}

// buildConnString assembles a PostgreSQL connection string from individual
// environment variables: LAW_DB_NAME, LAW_DB_USER, LAW_DB_PASS, LAW_DB_HOST,
// LAW_DB_PORT (default "5432"), and LAW_DB_PARAMS (optional).
func buildConnString() (string, error) {
	name := os.Getenv("LAW_DB_NAME")
	user := os.Getenv("LAW_DB_USER")
	pass := os.Getenv("LAW_DB_PASS")
	host := os.Getenv("LAW_DB_HOST")
	port := os.Getenv("LAW_DB_PORT")
	params := os.Getenv("LAW_DB_PARAMS")

	// If none of the individual vars are set, give the original error message.
	if name == "" && user == "" && pass == "" && host == "" {
		return "", errors.New("database connection string not set: set LAW_DBSTR or the LAW_DB_* variables")
	}

	// Check that all required vars are present.
	var missing []string
	if name == "" {
		missing = append(missing, "LAW_DB_NAME")
	}
	if user == "" {
		missing = append(missing, "LAW_DB_USER")
	}
	if pass == "" {
		missing = append(missing, "LAW_DB_PASS")
	}
	if host == "" {
		missing = append(missing, "LAW_DB_HOST")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required database environment variables: %s", strings.Join(missing, ", "))
	}

	if port == "" {
		port = "5432"
	}

	u := &url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(user, pass),
		Host:   host + ":" + port,
		Path:   name,
	}
	if params != "" {
		u.RawQuery = params
	}

	slog.Debug("assembled database connection string from LAW_DB_* variables", "host", host, "port", port, "name", name)

	return u.String(), nil
}
