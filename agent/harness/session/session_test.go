package session

import (
	"context"
	"testing"

	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// InMemorySessionStorage tests
// ============================================================================

func TestInMemoryStorage_Empty(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	meta, err := storage.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID == "" {
		t.Error("expected non-empty ID")
	}

	leafID, err := storage.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leafID != nil {
		t.Errorf("expected nil leaf, got %v", leafID)
	}

	entries, err := storage.GetEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestInMemoryStorage_AppendAndGetEntry(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	id, err := storage.CreateEntryID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("expected non-empty entry ID")
	}

	msg := ai.NewUserMessage("hello")
	err = storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type: "message",
			ID:   id,
			Timestamp: harness.NowISO(),
		},
		Message: &msg,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := storage.GetEntry(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected entry, got nil")
	}
	if got.Type != "message" {
		t.Errorf("expected type 'message', got %q", got.Type)
	}
	if got.Message == nil || got.Message.Role != "user" {
		t.Error("expected user message")
	}
}

func TestInMemoryStorage_LeafTracking(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	// Append a message — should become the leaf
	id, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id, Timestamp: harness.NowISO()},
	})

	leafID, err := storage.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leafID == nil || *leafID != id {
		t.Errorf("expected leaf=%s, got %s", id, derefStr(leafID))
	}
}

func TestInMemoryStorage_SetLeafID(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	// Append entries
	id1, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id1, Timestamp: harness.NowISO()},
	})
	id2, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id2, Timestamp: harness.NowISO()},
	})

	// Set leaf back to first entry
	err := storage.SetLeafID(ctx, &id1)
	if err != nil {
		t.Fatal(err)
	}

	leafID, err := storage.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leafID == nil || *leafID != id1 {
		t.Errorf("expected leaf=%s, got %s", id1, derefStr(leafID))
	}
}

func TestInMemoryStorage_FindEntries(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		id, _ := storage.CreateEntryID(ctx)
		storage.AppendEntry(ctx, harness.SessionTreeEntry{
			SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id, Timestamp: harness.NowISO()},
		})
	}
	id4, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "model_change", ID: id4, Timestamp: harness.NowISO()},
		Provider: "test",
		ModelID:  "gpt-4",
	})

	messages, err := storage.FindEntries(ctx, "message")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Errorf("expected 3 message entries, got %d", len(messages))
	}

	models, err := storage.FindEntries(ctx, "model_change")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Errorf("expected 1 model_change entry, got %d", len(models))
	}
}

func TestInMemoryStorage_Labels(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	// Add a message entry
	msgID, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: msgID, Timestamp: harness.NowISO()},
	})

	// Add a label for it
	labelID, _ := storage.CreateEntryID(ctx)
	labelText := "important"
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "label", ID: labelID, Timestamp: harness.NowISO()},
		TargetID:             &msgID,
		Label:                &labelText,
	})

	label, err := storage.GetLabel(ctx, msgID)
	if err != nil {
		t.Fatal(err)
	}
	if label == nil || *label != "important" {
		t.Errorf("expected label 'important', got %v", label)
	}
}

func TestInMemoryStorage_GetPathToRoot(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	// Build a chain: entry1 -> entry2 -> entry3
	id1, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id1, Timestamp: harness.NowISO()},
	})

	id2, _ := storage.CreateEntryID(ctx)
	parentID1 := id1
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id2, ParentID: &parentID1, Timestamp: harness.NowISO()},
	})

	id3, _ := storage.CreateEntryID(ctx)
	parentID2 := id2
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id3, ParentID: &parentID2, Timestamp: harness.NowISO()},
	})

	path, err := storage.GetPathToRoot(ctx, &id3)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 3 {
		t.Fatalf("expected path of length 3, got %d", len(path))
	}
	if path[0].ID != id1 {
		t.Errorf("expected path[0]=%s, got %s", id1, path[0].ID)
	}
	if path[2].ID != id3 {
		t.Errorf("expected path[2]=%s, got %s", id3, path[2].ID)
	}
}

// ============================================================================
// Session tests
// ============================================================================

func TestSession_AppendAndBuildContext(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	session := NewSession(storage)
	ctx := context.Background()

	// Append a user message
	_, err := session.AppendMessage(ctx, ai.NewUserMessage("hello"))
	if err != nil {
		t.Fatal(err)
	}

	// Append an assistant message
	_, err = session.AppendMessage(ctx, ai.Message{
		Role:      "assistant",
		Content:   "hi there",
		Provider:  "test",
		Model:     "test-model",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build context
	sctx, err := session.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sctx.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sctx.Messages))
	}
	if sctx.Messages[0].Role != "user" {
		t.Errorf("expected first message role=user, got %q", sctx.Messages[0].Role)
	}
	if sctx.Messages[1].Role != "assistant" {
		t.Errorf("expected second message role=assistant, got %q", sctx.Messages[1].Role)
	}
	if sctx.Model == nil || sctx.Model.Provider != "test" {
		t.Error("expected model provider=test from assistant message")
	}
}

func TestSession_AppendModelChange(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	session := NewSession(storage)
	ctx := context.Background()

	_, err := session.AppendModelChange(ctx, "openai", "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}

	sctx, err := session.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sctx.Model == nil {
		t.Fatal("expected model in context")
	}
	if sctx.Model.Provider != "openai" || sctx.Model.ModelID != "gpt-4o" {
		t.Errorf("expected openai/gpt-4o, got %s/%s", sctx.Model.Provider, sctx.Model.ModelID)
	}
}

func TestSession_AppendThinkingLevelChange(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	session := NewSession(storage)
	ctx := context.Background()

	_, err := session.AppendThinkingLevelChange(ctx, "high")
	if err != nil {
		t.Fatal(err)
	}

	sctx, err := session.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sctx.ThinkingLevel != "high" {
		t.Errorf("expected thinking level 'high', got %q", sctx.ThinkingLevel)
	}
}

func TestSession_AppendActiveToolsChange(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	session := NewSession(storage)
	ctx := context.Background()

	_, err := session.AppendActiveToolsChange(ctx, []string{"read_file", "write_file"})
	if err != nil {
		t.Fatal(err)
	}

	sctx, err := session.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sctx.ActiveToolNames) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(sctx.ActiveToolNames))
	}
	if sctx.ActiveToolNames[0] != "read_file" {
		t.Errorf("expected 'read_file', got %q", sctx.ActiveToolNames[0])
	}
}

func TestSession_AppendSessionName(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	session := NewSession(storage)
	ctx := context.Background()

	_, err := session.AppendSessionName(ctx, "test session")
	if err != nil {
		t.Fatal(err)
	}

	name, err := session.GetSessionName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name == nil || *name != "test session" {
		t.Errorf("expected 'test session', got %v", name)
	}
}

func TestSession_MoveTo(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	session := NewSession(storage)
	ctx := context.Background()

	// Add some entries
	id1, _ := session.AppendMessage(ctx, ai.NewUserMessage("hello"))
	_, _ = session.AppendMessage(ctx, ai.Message{Role: "assistant", Content: "hi", Timestamp: 1})

	// Move back to first entry
	summaryID, err := session.MoveTo(ctx, &id1, &harness.BranchSummaryResult{
		Summary:  "summarized branch",
		FromHook: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	leafID, err := session.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// After MoveTo with summary, leaf should be the branch summary entry
	// (the storage tracks the most recently appended entry as leaf)
	if leafID == nil {
		t.Fatal("expected non-nil leaf after MoveTo")
	}
	// The summary ID should be non-nil
	if summaryID == nil {
		t.Error("expected summary entry ID")
	}
}

func TestSession_AppendLabel_InvalidTarget(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	session := NewSession(storage)
	ctx := context.Background()

	_, err := session.AppendLabel(ctx, "nonexistent", strPtr("test"))
	if err == nil {
		t.Error("expected error for nonexistent target")
	}
}

// ============================================================================
// InMemorySessionRepo tests
// ============================================================================

func TestInMemoryRepo_CreateAndOpen(t *testing.T) {
	repo := NewInMemorySessionRepo()
	ctx := context.Background()

	session, err := repo.Create(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	meta, err := session.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID == "" {
		t.Error("expected non-empty session ID")
	}

	// Open it
	reopened, err := repo.Open(ctx, meta)
	if err != nil {
		t.Fatal(err)
	}
	reopenedMeta, _ := reopened.GetMetadata(ctx)
	if reopenedMeta.ID != meta.ID {
		t.Errorf("expected %s, got %s", meta.ID, reopenedMeta.ID)
	}
}

func TestInMemoryRepo_List(t *testing.T) {
	repo := NewInMemorySessionRepo()
	ctx := context.Background()

	repo.Create(ctx, nil)
	repo.Create(ctx, nil)

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(list))
	}
}

func TestInMemoryRepo_Delete(t *testing.T) {
	repo := NewInMemorySessionRepo()
	ctx := context.Background()

	session, _ := repo.Create(ctx, nil)
	meta, _ := session.GetMetadata(ctx)

	repo.Delete(ctx, meta)

	list, _ := repo.List(ctx)
	if len(list) != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", len(list))
	}
}

func TestInMemoryRepo_Open_NotFound(t *testing.T) {
	repo := NewInMemorySessionRepo()
	ctx := context.Background()

	_, err := repo.Open(ctx, harness.SessionMetadata{ID: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestInMemoryRepo_Fork(t *testing.T) {
	repo := NewInMemorySessionRepo()
	ctx := context.Background()

	// Create source session with messages
	session, _ := repo.Create(ctx, nil)
	session.AppendMessage(ctx, ai.NewUserMessage("hello"))
	session.AppendMessage(ctx, ai.Message{Role: "assistant", Content: "hi", Timestamp: 1})
	session.AppendMessage(ctx, ai.NewUserMessage("how are you"))

	sourceMeta, _ := session.GetMetadata(ctx)

	// Fork — should copy all entries
	forked, err := repo.Fork(ctx, sourceMeta, SessionForkOptions{})
	if err != nil {
		t.Fatal(err)
	}

	forkedCtx, err := forked.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(forkedCtx.Messages) != 3 {
		t.Errorf("expected 3 messages in forked session, got %d", len(forkedCtx.Messages))
	}

	// Fork should have different ID
	forkedMeta, _ := forked.GetMetadata(ctx)
	if forkedMeta.ID == sourceMeta.ID {
		t.Error("forked session should have different ID")
	}
}

// ============================================================================
// BuildSessionContext tests
// ============================================================================

func TestBuildSessionContext_Compaction(t *testing.T) {
	storage := NewInMemorySessionStorage(nil)
	ctx := context.Background()

	// Build entries: msg1, msg2, msg3, compaction(firstKept=msg2), msg4
	msg1 := ai.NewUserMessage("old1")
	msg2 := ai.NewUserMessage("kept1")
	msg3 := ai.NewUserMessage("kept2")
	msg4 := ai.NewUserMessage("new")

	id1, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id1, Timestamp: harness.NowISO()},
		Message: &msg1,
	})

	id2, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id2, Timestamp: harness.NowISO()},
		Message: &msg2,
	})

	id3, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id3, Timestamp: harness.NowISO()},
		Message: &msg3,
	})

	// Compaction: keep from id2 onwards
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "compaction", ID: "comp1", Timestamp: harness.NowISO()},
		Summary:          "old conversation about...",
		FirstKeptEntryID: id2,
		TokensBefore:     1000,
	})

	id5, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: id5, Timestamp: harness.NowISO()},
		Message: &msg4,
	})

	// Build context from all entries
	entries, _ := storage.GetEntries(ctx)
	sctx := BuildSessionContext(entries)

	// Should have: compaction summary + kept1 + kept2 + new = 4 messages
	if len(sctx.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(sctx.Messages))
	}

	// First should be compaction summary
	if sctx.Messages[0].Role != "user" {
		t.Errorf("expected first message to be user (compaction summary), got %q", sctx.Messages[0].Role)
	}

	// Second should be "kept1"
	content, ok := sctx.Messages[1].Content.(string)
	if !ok || content != "kept1" {
		t.Errorf("expected 'kept1', got %v", sctx.Messages[1].Content)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func strPtr(s string) *string { return &s }

func derefStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
