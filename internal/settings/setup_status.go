package settings

import (
	"context"
	"strings"
)

// SetupStatus is derived from loaded config, encryption key, and forge inventory —
// not only the setup_completed DB flag.
type SetupStatus struct {
	NeedsSetup           bool     `json:"needs_setup"`
	SetupCompleted       bool     `json:"setup_completed"`
	EncryptionConfigured bool     `json:"encryption_configured"`
	ForgeConfigured      bool     `json:"forge_configured"`
	FirstRun             bool     `json:"first_run"` // !setup_completed
	Reasons              []string `json:"reasons,omitempty"`
}

// EvaluateSetup inspects encryption and forge readiness from config file / env /
// wizard key file and DB instances. A true setup_completed flag alone is not enough:
// missing encryption or no usable forge still means NeedsSetup (un/misconfigured).
//
// When allowSkipSetup is true and setup was marked complete (local Skip Setup),
// NeedsSetup is false even without encryption/forge.
func (m *Manager) EvaluateSetup(ctx context.Context, allowSkipSetup bool) SetupStatus {
	st := SetupStatus{
		SetupCompleted:       m.SetupCompleted(),
		EncryptionConfigured: m.EncryptionConfigured(),
		ForgeConfigured:      m.HasUsableForge(ctx),
	}
	st.FirstRun = !st.SetupCompleted

	if allowSkipSetup && st.SetupCompleted {
		st.NeedsSetup = false
		return st
	}

	if !st.EncryptionConfigured {
		st.Reasons = append(st.Reasons, "encryption key not configured (GITSEER_ENCRYPTION_KEY, encryption_key_file, or wizard file)")
	}
	if !st.ForgeConfigured {
		st.Reasons = append(st.Reasons, "no forge with URL and API token (config or instances)")
		st.Reasons = append(st.Reasons, m.partialForgeHints()...)
	}
	if !st.SetupCompleted {
		st.Reasons = append(st.Reasons, "setup wizard not finished")
	}

	st.NeedsSetup = !st.EncryptionConfigured || !st.ForgeConfigured || !st.SetupCompleted
	return st
}

// NeedsSetup reports whether operators should run / resume the Setup Wizard.
func (m *Manager) NeedsSetup(ctx context.Context, allowSkipSetup bool) bool {
	return m.EvaluateSetup(ctx, allowSkipSetup).NeedsSetup
}

// HasUsableForge reports whether config or DB has at least one forge with a base
// URL and sync API credential (plaintext from config or sealed instance token).
func (m *Manager) HasUsableForge(ctx context.Context) bool {
	if m.AnyForgeConfigured() {
		return true
	}
	if m.st == nil {
		return false
	}
	list, err := m.st.ListInstances(ctx)
	if err != nil {
		return false
	}
	for i := range list {
		inst := &list[i]
		if strings.TrimSpace(inst.BaseURL) == "" {
			continue
		}
		if strings.TrimSpace(inst.SyncTokenCiphertext) != "" {
			return true
		}
	}
	return false
}

// partialForgeHints lists incomplete forge URL-without-token cases from config.
func (m *Manager) partialForgeHints() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var hints []string
	if u := strings.TrimSpace(m.integ.URL); u != "" && strings.TrimSpace(m.integ.Token) == "" {
		hints = append(hints, "gitea/forgejo URL set in config without token")
	}
	if u := strings.TrimSpace(m.github.URL); u != "" && strings.TrimSpace(m.github.Token) == "" {
		hints = append(hints, "github URL set in config without token")
	}
	return hints
}
