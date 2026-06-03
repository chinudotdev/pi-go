package sdk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinudotdev/pi-go/agent/harness/session"
	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/sdk/auth"
	"github.com/chinudotdev/pi-go/sdk/models"
	"github.com/chinudotdev/pi-go/sdk/resources"
	"github.com/chinudotdev/pi-go/sdk/settings"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pi-sdk-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func newTestDeps(t *testing.T) (*auth.Storage, *settings.Manager, *models.Registry) {
	backend := auth.NewMemoryBackend()
	authStorage := auth.NewStorage(backend)
	settingsMgr := settings.InMemory()
	modelReg := models.InMemory(authStorage)
	return authStorage, settingsMgr, modelReg
}

func TestCreateSession_BasicFields(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if result.Session == nil {
		t.Fatal("expected non-nil session")
	}
	if result.Session.CWD() != dir {
		t.Errorf("expected CWD %s, got %s", dir, result.Session.CWD())
	}
	if result.Session.Settings() != settingsMgr {
		t.Error("expected settings manager to be passed through")
	}
	if result.Session.ModelRegistry() != modelReg {
		t.Error("expected model registry to be passed through")
	}
}

func TestCreateSession_NoTools(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	names := result.Session.GetActiveToolNames()
	if len(names) != 0 {
		t.Errorf("expected no tools when NoTools=true, got %v", names)
	}

	if result.Session.SystemPrompt() == "" {
		t.Error("expected non-empty system prompt")
	}
}

func TestCreateSession_SystemPromptWithContextFile(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	writeFile(t, filepath.Join(dir, "AGENTS.md"), "Always write tests")

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	resLoader := resources.NewLoader(resources.LoaderOptions{
		CWD:      dir,
		AgentDir: dir,
	})

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:            dir,
		AgentDir:       dir,
		AuthStorage:    authStorage,
		SettingsMgr:    settingsMgr,
		ModelRegistry:  modelReg,
		SessionDir:     sessDir,
		NoTools:        true,
		ResourceLoader: resLoader,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	sp := result.Session.SystemPrompt()
	if sp == "" {
		t.Fatal("expected non-empty system prompt")
	}
	if !strings.Contains(sp, "Always write tests") {
		t.Errorf("expected system prompt to include AGENTS.md content")
	}
}

func TestCreateSession_NoModel_FallbackMessage(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if result.ModelFallbackMessage == "" {
		t.Error("expected fallback message when no models available")
	}
	if result.Session.Model() != nil {
		t.Error("expected nil model when no models available")
	}
}

func TestCreateSession_ThinkingLevel(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
		ThinkingLevel: "high",
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	// Without a model, thinking level should be clamped to "off"
	if result.Session.ThinkingLevel() != ai.ThinkingOff {
		t.Errorf("expected thinking level 'off' (no model), got %q", result.Session.ThinkingLevel())
	}
}

func TestCreateSession_ResourceAccess(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	skills := result.Session.GetSkills()
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}

	contextFiles := result.Session.GetContextFiles()
	if len(contextFiles) != 0 {
		t.Errorf("expected 0 context files, got %d", len(contextFiles))
	}

	templates := result.Session.GetPromptTemplates()
	if len(templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(templates))
	}
}

func TestCreateSession_Dispose(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	result.Session.Dispose(ctx) // should not panic
}

func TestCreateSession_HarnessAndSessionAccess(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Deprecated escape hatches still work but are discouraged
	if result.Session.Harness() == nil {
		t.Error("expected non-nil harness")
	}
	if result.Session.Session() == nil {
		t.Error("expected non-nil session")
	}
}

func TestCreateSession_SessionStats(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	stats, err := result.Session.GetSessionStats(ctx)
	if err != nil {
		t.Fatalf("GetSessionStats failed: %v", err)
	}
	if stats.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if stats.StartedAt.IsZero() {
		t.Error("expected non-zero StartedAt")
	}
	if stats.Duration() < 0 {
		t.Error("expected positive duration")
	}
	if stats.TotalMessages != 0 {
		t.Errorf("expected 0 messages in fresh session, got %d", stats.TotalMessages)
	}
}

func TestCreateSession_WithCustomResourceLoader(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	writeFile(t, filepath.Join(dir, "SYSTEM.md"), "Custom system prompt from file")

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	resLoader := resources.NewLoader(resources.LoaderOptions{
		CWD:      dir,
		AgentDir: dir,
	})

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:            dir,
		AgentDir:       dir,
		AuthStorage:    authStorage,
		SettingsMgr:    settingsMgr,
		ModelRegistry:  modelReg,
		SessionDir:     sessDir,
		NoTools:        true,
		ResourceLoader: resLoader,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	sp := result.Session.SystemPrompt()
	if !strings.Contains(sp, "Custom system prompt from file") {
		t.Errorf("expected system prompt to contain custom prompt")
	}
}

func TestCompactSettingsFromManager(t *testing.T) {
	mgr := settings.InMemory()
	cs := compactionSettingsFromManager(mgr)
	if !cs.Enabled {
		t.Error("expected compaction enabled by default")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()
	if id1 == id2 {
		t.Error("expected unique session IDs")
	}
	if !strings.Contains(id1, "sess_") {
		t.Errorf("expected session ID to contain sess_, got %q", id1)
	}
}

func TestBuildSystemPromptFromResources(t *testing.T) {
	dir := tempDir(t)

	loadedRes := &resources.LoadedResources{
		ContextFiles: []resources.ContextFile{
			{Path: filepath.Join(dir, "AGENTS.md"), Content: "Use Go"},
		},
	}

	toolRes := &toolResolution{
		ActiveNames: []string{"read", "bash"},
	}

	sp := buildSystemPromptFromResources(dir, loadedRes, toolRes)
	if !strings.Contains(sp, "Use Go") {
		t.Error("expected prompt to contain context file content")
	}
}

func TestCreateSession_DefaultTools(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		// NoTools not set — should create default tools
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	names := result.Session.GetActiveToolNames()
	if len(names) == 0 {
		t.Error("expected default tools to be created")
	}

	// Should contain at least read and bash
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["read"] || !nameSet["bash"] {
		t.Errorf("expected read and bash in default tools, got %v", names)
	}
}

func TestSubscribe_Unsubscribe(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	sess := result.Session

	var callCount int
	listener := func(event SessionEvent) {
		callCount++
	}

	// Subscribe and immediately unsubscribe
	unsub := sess.Subscribe(listener)
	unsub()

	// Subscribe two listeners, unsubscribe first
	_ = sess.Subscribe(listener)
	unsub2 := sess.Subscribe(listener)
	unsub2()

	// Dispose should work fine with remaining listener
	sess.Dispose(ctx)
}

func TestSubscribe_MultipleUnsubscribe(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	listener1 := func(event SessionEvent) {}
	listener2 := func(event SessionEvent) {}

	unsub1 := result.Session.Subscribe(listener1)
	unsub2 := result.Session.Subscribe(listener2)

	// Unsubscribe first listener
	unsub1()

	// Second should still be subscribed (no panic)
	unsub2()

	// Double unsubscribe should be safe
	unsub1()
}

func TestCreateSession_NoToolsAndToolListConflict(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	_, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
		ToolList:      []string{"bash"},
	})
	if err == nil {
		t.Fatal("expected error when NoTools and ToolList are both set")
	}
	if !strings.Contains(err.Error(), "NoTools") {
		t.Errorf("expected NoTools conflict error, got: %v", err)
	}
}

func TestCreateSession_ToolListAndExcludeConflict(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	_, err := CreateSession(ctx, CreateSessionOptions{
		CWD:          dir,
		AgentDir:     dir,
		AuthStorage:  authStorage,
		SettingsMgr:  settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:   sessDir,
		ToolList:     []string{"bash"},
		ExcludeTools: []string{"read"},
	})
	if err == nil {
		t.Fatal("expected error when ToolList and ExcludeTools are both set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestCreateSession_NoToolsAndExcludeConflict(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	_, err := CreateSession(ctx, CreateSessionOptions{
		CWD:          dir,
		AgentDir:     dir,
		AuthStorage:  authStorage,
		SettingsMgr:  settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:   sessDir,
		NoTools:      true,
		ExcludeTools: []string{"read"},
	})
	if err == nil {
		t.Fatal("expected error when NoTools and ExcludeTools are both set")
	}
}

func TestCycleThinkingLevel_NoModel(t *testing.T) {
	dir := tempDir(t)
	sessDir := filepath.Join(dir, "sessions")
	os.MkdirAll(sessDir, 0755)

	authStorage, settingsMgr, modelReg := newTestDeps(t)
	ctx := context.Background()

	result, err := CreateSession(ctx, CreateSessionOptions{
		CWD:           dir,
		AgentDir:      dir,
		AuthStorage:   authStorage,
		SettingsMgr:   settingsMgr,
		ModelRegistry: modelReg,
		SessionDir:    sessDir,
		NoTools:       true,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// No model → should return empty string
	next := result.Session.CycleThinkingLevel(ctx)
	if next != "" {
		t.Errorf("expected empty string for cycle with no model, got %q", next)
	}
}

// Verify session.NewSession is reachable
var _ = session.NewSession
