package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestRerunCancelWorkflowRun(t *testing.T) {
	var rerun, cancel bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/acme/app/actions/runs/9/rerun", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		rerun = true
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/v1/repos/acme/app/actions/runs/9/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		cancel = true
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/api/v1/repos/acme/app/actions/runs/10/rerun", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, "tok", true)
	if err != nil {
		t.Fatal(err)
	}
	ref := models.RepoRef{Owner: "acme", Name: "app"}
	ctx := context.Background()
	if err := client.RerunWorkflowRun(ctx, ref, 9); err != nil {
		t.Fatal(err)
	}
	if err := client.CancelWorkflowRun(ctx, ref, 9); err != nil {
		t.Fatal(err)
	}
	if !rerun || !cancel {
		t.Fatalf("rerun=%v cancel=%v", rerun, cancel)
	}
	err = client.RerunWorkflowRun(ctx, ref, 10)
	if !forge.IsUnsupported(err) {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
}
