package db

import (
	"os"
	"strings"
	"testing"
)

func Test_EmptyConnString(t *testing.T) {
	os.Unsetenv("LAW_DBSTR")
	_, err := getConnString()
	if err == nil {
		t.Error("Did not get an error when DB connection string was not set")
	}
}

func Test_SetConnString(t *testing.T) {
	connstr := "postgres://user:pass@localhost:5432/cchc"
	envvar := "LAW_DBSTR"
	os.Setenv(envvar, connstr)
	got, err := getConnString()
	if err != nil {
		t.Error("Got an error when connection string was set: ", err)
	}
	if got != connstr {
		t.Error("Connection string does not match environment variable")
	}
}

// clearDBEnv unsets all database-related environment variables.
func clearDBEnv() {
	os.Unsetenv("LAW_DBSTR")
	os.Unsetenv("LAW_DB_NAME")
	os.Unsetenv("LAW_DB_USER")
	os.Unsetenv("LAW_DB_PASS")
	os.Unsetenv("LAW_DB_HOST")
	os.Unsetenv("LAW_DB_PORT")
	os.Unsetenv("LAW_DB_PARAMS")
}

func Test_SplitConnString(t *testing.T) {
	clearDBEnv()
	os.Setenv("LAW_DB_NAME", "testdb")
	os.Setenv("LAW_DB_USER", "admin")
	os.Setenv("LAW_DB_PASS", "secret")
	os.Setenv("LAW_DB_HOST", "db.example.com")
	os.Setenv("LAW_DB_PORT", "5433")

	got, err := getConnString()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "postgresql://admin:secret@db.example.com:5433/testdb"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func Test_SplitConnStringDefaultPort(t *testing.T) {
	clearDBEnv()
	os.Setenv("LAW_DB_NAME", "testdb")
	os.Setenv("LAW_DB_USER", "admin")
	os.Setenv("LAW_DB_PASS", "secret")
	os.Setenv("LAW_DB_HOST", "localhost")

	got, err := getConnString()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "postgresql://admin:secret@localhost:5432/testdb"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func Test_SplitConnStringWithParams(t *testing.T) {
	clearDBEnv()
	os.Setenv("LAW_DB_NAME", "testdb")
	os.Setenv("LAW_DB_USER", "admin")
	os.Setenv("LAW_DB_PASS", "secret")
	os.Setenv("LAW_DB_HOST", "localhost")
	os.Setenv("LAW_DB_PARAMS", "connect_timeout=15&pool_max_conns=8")

	got, err := getConnString()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "postgresql://admin:secret@localhost:5432/testdb?connect_timeout=15&pool_max_conns=8"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func Test_SplitConnStringWithoutParams(t *testing.T) {
	clearDBEnv()
	os.Setenv("LAW_DB_NAME", "testdb")
	os.Setenv("LAW_DB_USER", "admin")
	os.Setenv("LAW_DB_PASS", "secret")
	os.Setenv("LAW_DB_HOST", "localhost")

	got, err := getConnString()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "?") {
		t.Errorf("expected no query string, got %q", got)
	}
}

func Test_SplitConnStringMissingRequired(t *testing.T) {
	clearDBEnv()
	os.Setenv("LAW_DB_NAME", "testdb")
	// LAW_DB_USER, LAW_DB_PASS, LAW_DB_HOST are missing

	_, err := getConnString()
	if err == nil {
		t.Fatal("expected an error for missing required vars")
	}
	for _, name := range []string{"LAW_DB_USER", "LAW_DB_PASS", "LAW_DB_HOST"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error should mention %s, got: %v", name, err)
		}
	}
}

func Test_SplitConnStringSpecialChars(t *testing.T) {
	clearDBEnv()
	os.Setenv("LAW_DB_NAME", "testdb")
	os.Setenv("LAW_DB_USER", "user@org")
	os.Setenv("LAW_DB_PASS", "p@ss:w0rd/!")
	os.Setenv("LAW_DB_HOST", "localhost")

	got, err := getConnString()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Special characters must be percent-encoded in the URL.
	if strings.Contains(got, "user@org@") {
		t.Errorf("user '@' not encoded: %q", got)
	}
	if strings.Contains(got, "p@ss:w0rd/!") {
		t.Errorf("password special chars not encoded: %q", got)
	}
	// The result should still be a valid connection string that contains the host.
	if !strings.Contains(got, "@localhost:") {
		t.Errorf("expected @localhost: in result, got %q", got)
	}
}

func Test_LAW_DBSTR_TakesPrecedence(t *testing.T) {
	clearDBEnv()
	os.Setenv("LAW_DBSTR", "postgres://fromdbstr@host:5432/db")
	os.Setenv("LAW_DB_NAME", "other")
	os.Setenv("LAW_DB_USER", "other")
	os.Setenv("LAW_DB_PASS", "other")
	os.Setenv("LAW_DB_HOST", "otherhost")

	got, err := getConnString()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "postgres://fromdbstr@host:5432/db"
	if got != expected {
		t.Errorf("LAW_DBSTR should take precedence: got %q, want %q", got, expected)
	}
}
