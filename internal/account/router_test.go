package account

import (
	"testing"
	"time"
)

func TestRouterSessionAffinityAndCooldown(t *testing.T) {
	now := time.Date(2026,9,22,0,0,0,0,time.UTC)
	r := NewRouter([]Credential{{ID:"a",Token:"ta",DeviceID:"da"},{ID:"b",Token:"tb",DeviceID:"db"}})
	r.now = func() time.Time { return now }

	first := r.Select("session-1","")
	if first == nil || first.Credential.ID != "a" { t.Fatalf("expected a, got %#v",first) }
	again := r.Select("session-1","")
	if again == nil || again.Credential.ID != "a" { t.Fatalf("session affinity lost: %#v",again) }

	r.MarkRateLimited("a",now.Add(5*time.Minute),"weekly limit")
	failover := r.Select("session-1","")
	if failover == nil || failover.Credential.ID != "b" { t.Fatalf("expected failover to b, got %#v",failover) }

	now = now.Add(6*time.Minute)
	other := r.Select("session-2","a")
	if other == nil || other.Credential.ID != "a" || other.State != StateHealthy { t.Fatalf("cooldown did not recover: %#v",other) }
}

func TestRouterSuspendedAndDisabled(t *testing.T) {
	r := NewRouter([]Credential{{ID:"a",DeviceID:"da"},{ID:"b",DeviceID:"db"}})
	r.MarkSuspended("a","policy")
	if got:=r.Select("s","a"); got!=nil { t.Fatalf("suspended account selected: %#v",got) }
	if got:=r.Select("s",""); got==nil || got.Credential.ID!="b" { t.Fatalf("expected b, got %#v",got) }
	r.SetDisabled("b",true)
	if got:=r.Select("s2",""); got!=nil { t.Fatalf("expected no eligible account, got %#v",got) }
}


func TestManualDisableIsIndependentFromRuntimeState(t *testing.T) {
	r := NewRouter([]Credential{{ID:"a",Token:"ta",DeviceID:"da"}})
	r.MarkSuspended("a","80006")
	r.SetDisabled("a",true)

	snapshot:=r.Snapshot()
	if len(snapshot)!=1 || !snapshot[0].Credential.Disabled {
		t.Fatalf("manual disabled state not set: %#v",snapshot)
	}
	if snapshot[0].State!=StateSuspended {
		t.Fatalf("manual disable must preserve runtime suspension, got %s",snapshot[0].State)
	}
	if got:=r.Select("s","a"); got!=nil {
		t.Fatalf("manually disabled account must not be selected: %#v",got)
	}

	r.SetDisabled("a",false)
	snapshot=r.Snapshot()
	if snapshot[0].Credential.Disabled {
		t.Fatal("manual enable did not clear disabled flag")
	}
	if snapshot[0].State!=StateSuspended {
		t.Fatalf("manual enable must not erase suspension, got %s",snapshot[0].State)
	}
	if got:=r.Select("s","a"); got!=nil {
		t.Fatalf("suspended account must remain ineligible after manual enable: %#v",got)
	}
}
