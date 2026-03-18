package telegram

import (
	"context"
	"testing"
	"time"
)

func TestApprovalBroker_ApproveFlow(t *testing.T) {
	broker := NewApprovalBroker()
	approveData, _, err := broker.Register("tc_1", "chat_1", time.Second)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	waitCh := make(chan bool, 1)
	go func() {
		resp, waitErr := broker.Wait(context.Background(), "tc_1")
		if waitErr != nil || resp == nil {
			waitCh <- false
			return
		}
		waitCh <- resp.Approved
	}()
	time.Sleep(20 * time.Millisecond)

	approved, err := broker.ResolveByCallback("chat_1", approveData)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !approved {
		t.Fatalf("expected approved=true")
	}

	select {
	case ok := <-waitCh:
		if !ok {
			t.Fatalf("wait result mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("wait timed out")
	}
}

func TestApprovalBroker_RejectCrossChat(t *testing.T) {
	broker := NewApprovalBroker()
	approveData, _, err := broker.Register("tc_2", "chat_owner", time.Second)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = broker.ResolveByCallback("chat_other", approveData)
	if err == nil {
		t.Fatal("expected cross chat resolve error")
	}
}

func TestApprovalBroker_RejectExpiredToken(t *testing.T) {
	broker := NewApprovalBroker()
	now := time.Now()
	broker.now = func() time.Time { return now }
	approveData, _, err := broker.Register("tc_3", "chat_3", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	broker.now = func() time.Time { return now.Add(500 * time.Millisecond) }
	_, err = broker.ResolveByCallback("chat_3", approveData)
	if err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestApprovalBroker_RejectInvalidToken(t *testing.T) {
	broker := NewApprovalBroker()
	_, _, err := broker.Register("tc_4", "chat_4", time.Second)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = broker.ResolveByCallback("chat_4", "ap:invalidtoken")
	if err == nil {
		t.Fatal("expected invalid token error")
	}
}
