package main

import "testing"

func TestResolveFrontendBaseURLAcceptsLegacyFrontendURL(t *testing.T) {
	got := resolveFrontendBaseURL(func(k string) string {
		if k == "FRONTEND_URL" {
			return " https://frontend.example.com/ "
		}
		return ""
	})
	if got != "https://frontend.example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveFrontendBaseURLPrefersLegacyFrontendURL(t *testing.T) {
	got := resolveFrontendBaseURL(func(k string) string {
		switch k {
		case "FRONTEND_URL":
			return "https://legacy.example.com"
		case "RCP_FRONTEND_BASE_URL":
			return "https://rcp.example.com"
		default:
			return ""
		}
	})
	if got != "https://legacy.example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveDBDSNDefaultsToCanonicalSQLitePath(t *testing.T) {
	driver, dsn := resolveDBConfig(func(string) string { return "" })
	if driver != "sqlite3" {
		t.Fatalf("driver got %q", driver)
	}
	if dsn != "file:/var/lib/rcp/rcp.db?cache=shared&_pragma=foreign_keys(1)" {
		t.Fatalf("dsn got %q", dsn)
	}
}
