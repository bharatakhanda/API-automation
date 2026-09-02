package application

import (
	"testing"

	"api-automation/internal/model"
)

func TestConnectionStateRequiresExactTestedDraftBeforeApply(t *testing.T) {
	state := NewConnectionState("configured-key")
	draft := state.ResolveDraft(model.ServerConnection{IPAddress: " fiery.example ", Password: " admin "})
	if draft.SecretKey != "configured-key" || draft.IPAddress != "fiery.example" || draft.Password != "admin" {
		t.Fatalf("resolved draft = %#v", draft)
	}
	state.BeginTest()
	state.CompleteTest(draft, true, "Connection OK · press OK to apply")
	changedDraft := draft
	changedDraft.Password = "different"
	if !state.InvalidateIfChanged(changedDraft) || state.Snapshot().TestOK {
		t.Fatal("changed credentials did not invalidate exact-draft approval")
	}
	if _, _, err := state.Apply(draft); err == nil {
		t.Fatal("invalidated draft was applied")
	}
	state.CompleteTest(draft, true, "Connection OK · press OK to apply")
	active, changed, err := state.Apply(draft)
	if err != nil || !changed || active != draft {
		t.Fatalf("active=%#v changed=%t err=%v", active, changed, err)
	}
	if state.Snapshot().TestOK {
		t.Fatal("test approval survived application")
	}
}

func TestConnectionStateKeepsCredentialsOutOfReplacementDraft(t *testing.T) {
	active := model.ServerConnection{IPAddress: "fiery.example", SecretKey: "configured-key", Password: "configured-password"}
	state := NewConnectionStateWithActive("ignored-key", active)
	resolved := state.ResolveDraft(model.ServerConnection{IPAddress: "fiery.example"})
	if resolved.SecretKey != active.SecretKey || resolved.Password != active.Password {
		t.Fatalf("blank replacements did not resolve internally: %#v", resolved)
	}
	snapshot := state.Snapshot()
	if !snapshot.SecretConfigured || !snapshot.PasswordConfigured || snapshot.ActiveIPAddress != "fiery.example" {
		t.Fatalf("safe snapshot = %#v", snapshot)
	}
}
