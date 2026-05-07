package codexappserver_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	codexappserver "github.com/zchee/omxx/pkg/codex-app-server"
)

const runRealCodexTestsEnv = "RUN_REAL_CODEX_TESTS"

func TestRealCodexAppServerInitializeAndModelList(t *testing.T) {
	if os.Getenv(runRealCodexTestsEnv) != "1" {
		t.Skipf("set %s=1 to run real Codex app-server integration coverage", runRealCodexTestsEnv)
	}

	codexBin, err := exec.LookPath("codex")
	if err != nil {
		t.Skipf("real Codex app-server integration requires codex binary on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	codex, err := codexappserver.NewCodex(ctx, &codexappserver.Config{CodexBin: codexBin})
	if err != nil {
		t.Fatalf("NewCodex() with real app-server error = %v", err)
	}
	defer func() {
		if err := codex.Close(); err != nil {
			t.Fatalf("Close() real app-server error = %v", err)
		}
	}()

	metadata := codex.Metadata()
	if strings.TrimSpace(metadata.UserAgent) == "" {
		t.Fatalf("Metadata().UserAgent is empty: %#v", metadata)
	}
	if metadata.ServerInfo == nil {
		t.Fatalf("Metadata().ServerInfo is nil: %#v", metadata)
	}
	if strings.TrimSpace(metadata.ServerInfo.Name) == "" {
		t.Fatalf("Metadata().ServerInfo.Name is empty: %#v", metadata.ServerInfo)
	}
	if strings.TrimSpace(metadata.ServerInfo.Version) == "" {
		t.Fatalf("Metadata().ServerInfo.Version is empty: %#v", metadata.ServerInfo)
	}

	models, err := codex.Models(ctx, true)
	if err != nil {
		t.Fatalf("Models(includeHidden=true) real app-server error = %v", err)
	}
	for index, model := range models.Data {
		if strings.TrimSpace(model.ID) == "" {
			t.Fatalf("Models().Data[%d].ID is empty: %#v", index, model)
		}
	}
}
