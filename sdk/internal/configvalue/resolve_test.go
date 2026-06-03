package configvalue

import (
	"os"
	"testing"
)

func TestResolveLiteral(t *testing.T) {
	result := ResolveConfigValue("hello-world")
	assertString(t, result, "hello-world")
}

func TestResolveEnvVar(t *testing.T) {
	os.Setenv("TEST_PI_KEY", "secret123")
	defer os.Unsetenv("TEST_PI_KEY")

	result := ResolveConfigValue("$TEST_PI_KEY")
	assertString(t, result, "secret123")
}

func TestResolveEnvVarBraces(t *testing.T) {
	os.Setenv("TEST_PI_KEY2", "secret456")
	defer os.Unsetenv("TEST_PI_KEY2")

	result := ResolveConfigValue("${TEST_PI_KEY2}")
	assertString(t, result, "secret456")
}

func TestResolveEnvVarMissing(t *testing.T) {
	result := ResolveConfigValue("$NONEXISTENT_PI_VAR_XYZ")
	if result != nil {
		t.Errorf("expected nil for missing env var, got %q", *result)
	}
}

func TestResolveTemplateMixed(t *testing.T) {
	os.Setenv("TEST_PI_HOST", "api.example.com")
	defer os.Unsetenv("TEST_PI_HOST")

	result := ResolveConfigValue("https://${TEST_PI_HOST}/v1")
	assertString(t, result, "https://api.example.com/v1")
}

func TestResolveTemplateMissingEnvVar(t *testing.T) {
	result := ResolveConfigValue("https://${NONEXISTENT_PI_HOST_XYZ}/v1")
	if result != nil {
		t.Errorf("expected nil for missing env var in template, got %q", *result)
	}
}

func TestResolveEscapedDollar(t *testing.T) {
	result := ResolveConfigValue("price is $$100")
	assertString(t, result, "price is $100")
}

func TestResolveEscapedBang(t *testing.T) {
	result := ResolveConfigValue("hello$!world")
	assertString(t, result, "hello!world")
}

func TestResolveCommand(t *testing.T) {
	ClearCache()
	result := ResolveConfigValue("!echo hello")
	assertString(t, result, "hello")
}

func TestResolveCommandCached(t *testing.T) {
	ClearCache()

	// First call
	result1 := ResolveConfigValue("!echo cached")
	assertString(t, result1, "cached")

	// Change what the command would output — cache should still return old value
	result2 := ResolveConfigValue("!echo cached")
	assertString(t, result2, "cached")
}

func TestResolveCommandFailure(t *testing.T) {
	ClearCache()
	result := ResolveConfigValue("!false")
	if result != nil {
		t.Errorf("expected nil for failing command, got %q", *result)
	}
}

func TestResolveCommandUncached(t *testing.T) {
	ClearCache()
	result := ResolveConfigValueUncached("!echo uncached")
	assertString(t, result, "uncached")
}

func TestGetConfigValueEnvVarName(t *testing.T) {
	name := GetConfigValueEnvVarName("$MY_KEY")
	if name == nil || *name != "MY_KEY" {
		t.Errorf("expected MY_KEY, got %v", name)
	}

	name = GetConfigValueEnvVarName("literal")
	if name != nil {
		t.Errorf("expected nil for literal, got %v", *name)
	}

	name = GetConfigValueEnvVarName("!echo foo")
	if name != nil {
		t.Errorf("expected nil for command, got %v", *name)
	}

	name = GetConfigValueEnvVarName("${MY_KEY}")
	if name == nil || *name != "MY_KEY" {
		t.Errorf("expected MY_KEY, got %v", name)
	}
}

func TestGetConfigValueEnvVarNames(t *testing.T) {
	names := GetConfigValueEnvVarNames("$A and $B and $A")
	if len(names) != 2 || names[0] != "A" || names[1] != "B" {
		t.Errorf("expected [A B], got %v", names)
	}
}

func TestGetMissingConfigValueEnvVarNames(t *testing.T) {
	os.Setenv("TEST_PI_EXISTS", "yes")
	defer os.Unsetenv("TEST_PI_EXISTS")

	missing := GetMissingConfigValueEnvVarNames("$TEST_PI_EXISTS and $TEST_PI_MISSING_XYZ")
	if len(missing) != 1 || missing[0] != "TEST_PI_MISSING_XYZ" {
		t.Errorf("expected [TEST_PI_MISSING_XYZ], got %v", missing)
	}
}

func TestIsCommandConfigValue(t *testing.T) {
	if !IsCommandConfigValue("!echo foo") {
		t.Error("expected true for command")
	}
	if IsCommandConfigValue("$VAR") {
		t.Error("expected false for env var")
	}
	if IsCommandConfigValue("literal") {
		t.Error("expected false for literal")
	}
}

func TestIsConfigValueConfigured(t *testing.T) {
	os.Setenv("TEST_PI_CONF", "yes")
	defer os.Unsetenv("TEST_PI_CONF")

	if !IsConfigValueConfigured("$TEST_PI_CONF") {
		t.Error("expected true for configured env var")
	}
	if IsConfigValueConfigured("$NONEXISTENT_PI_XYZ") {
		t.Error("expected false for missing env var")
	}
}

func TestIsLegacyEnvVarName(t *testing.T) {
	if !IsLegacyEnvVarName("MY_API_KEY") {
		t.Error("expected true for legacy env var name")
	}
	if IsLegacyEnvVarName("my_api_key") {
		t.Error("expected false for lowercase env var name")
	}
}

func TestResolveHeaders(t *testing.T) {
	os.Setenv("TEST_PI_AUTH", "Bearer token123")
	defer os.Unsetenv("TEST_PI_AUTH")

	headers := map[string]string{
		"Authorization": "$TEST_PI_AUTH",
		"X-Missing":     "$NONEXISTENT_PI_HEADER_XYZ",
	}
	resolved := ResolveHeaders(headers)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved header, got %d", len(resolved))
	}
	if resolved["Authorization"] != "Bearer token123" {
		t.Errorf("expected Bearer token123, got %s", resolved["Authorization"])
	}
}

func TestResolveHeadersOrThrow(t *testing.T) {
	os.Setenv("TEST_PI_THROW", "value123")
	defer os.Unsetenv("TEST_PI_THROW")

	headers := map[string]string{
		"X-Test": "$TEST_PI_THROW",
	}
	resolved := ResolveHeadersOrThrow(headers, "test")
	if resolved["X-Test"] != "value123" {
		t.Errorf("expected value123, got %s", resolved["X-Test"])
	}
}

func TestResolveHeadersOrThrowMissing(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for missing env var")
		}
	}()

	headers := map[string]string{
		"X-Test": "$NONEXISTENT_PI_THROW_XYZ",
	}
	ResolveHeadersOrThrow(headers, "test")
}

func TestResolveConfigValueOrThrow(t *testing.T) {
	os.Setenv("TEST_PI_OR_THROW", "works")
	defer os.Unsetenv("TEST_PI_OR_THROW")

	result := ResolveConfigValueOrThrow("$TEST_PI_OR_THROW", "test key")
	if result != "works" {
		t.Errorf("expected works, got %s", result)
	}
}

func TestResolveConfigValueOrThrowMissing(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for missing env var")
		}
	}()
	ResolveConfigValueOrThrow("$NONEXISTENT_PI_OR_THROW_XYZ", "test key")
}

// Helper
func assertString(t *testing.T, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("expected %q, got nil", want)
	} else if *got != want {
		t.Errorf("expected %q, got %q", want, *got)
	}
}
