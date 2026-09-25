package forge

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ncdlabs/gitseer/internal/models"
)

// Options configures a forge client. Sync and settings call New with these fields
// (token is plaintext; Instance stores ciphertext separately).
type Options struct {
	ForgeType           string
	BaseURL             string
	Token               string
	AllowPrivateNetwork bool
}

// Constructor builds a Forge from connection settings.
type Constructor func(baseURL, token string, allowPrivateNetwork bool) (Forge, error)

var (
	constructorsMu sync.RWMutex
	constructors   = map[string]Constructor{}
)

// Register associates a forge_type string with a constructor.
// Implementations register from init (see forge/gitea and forge/github).
func Register(forgeType string, c Constructor) {
	ft := strings.ToLower(strings.TrimSpace(forgeType))
	if ft == "" || c == nil {
		return
	}
	constructorsMu.Lock()
	defer constructorsMu.Unlock()
	constructors[ft] = c
}

// New returns a Forge for the given forge_type (gitea | github).
// Blank forge_type defaults to gitea. Callers must import the implementation
// packages (or forge/all) so constructors are registered.
func New(opts Options) (Forge, error) {
	ft := strings.ToLower(strings.TrimSpace(opts.ForgeType))
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	constructorsMu.RLock()
	c, ok := constructors[ft]
	constructorsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported forge type %q (import the forge implementation package)", opts.ForgeType)
	}
	return c(opts.BaseURL, opts.Token, opts.AllowPrivateNetwork)
}

// NewFromInstance builds a Forge from a stored instance row and decrypted token.
func NewFromInstance(inst models.Instance, token string) (Forge, error) {
	return New(Options{
		ForgeType:           inst.ForgeType,
		BaseURL:             inst.BaseURL,
		Token:               token,
		AllowPrivateNetwork: inst.AllowPrivateNetwork,
	})
}
