package gitea

import (
	"encoding/json"
	"testing"
)

func TestNewWebhookPreview(t *testing.T) {
	p := NewWebhookPreview("https://lens.example.com/api/webhooks/gitea", "")
	if p.Type != "gitea" || !p.Active {
		t.Fatalf("type/active = %s %v", p.Type, p.Active)
	}
	if p.Config["url"] != "https://lens.example.com/api/webhooks/gitea" {
		t.Fatalf("url = %q", p.Config["url"])
	}
	if p.Config["secret"] != "«generated on confirm»" {
		t.Fatalf("placeholder secret = %q", p.Config["secret"])
	}
	if p.Config["is_system_webhook"] != "true" {
		t.Fatal("expected system webhook")
	}
	for _, ev := range []string{"pull_request", "workflow_run", "workflow_job", "repository"} {
		found := false
		for _, e := range p.Events {
			if e == ev {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing event %s", ev)
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("invalid json")
	}
}

func TestGenerateWebhookSecret(t *testing.T) {
	a, err := GenerateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || len(b) != 64 {
		t.Fatalf("len a=%d b=%d", len(a), len(b))
	}
	if a == b {
		t.Fatal("secrets should differ")
	}
}

func TestAllRequiredOK(t *testing.T) {
	if !allRequiredOK([]ProbeCheck{{Status: ProbeOK}, {Status: ProbeSkip}, {Status: ProbeWarn}}) {
		t.Fatal("expected ok")
	}
	if allRequiredOK([]ProbeCheck{{Status: ProbeOK}, {Status: ProbeFail}}) {
		t.Fatal("expected fail")
	}
}
